package mrim

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	mraBufSize = 32768

	DefaultTimeout = 15 * time.Minute
)

var (
	errNoHello       = errors.New("no hello")
	errUnknownPacket = errors.New("unknown packet")
)

func Dial(ctx context.Context, addr string) (*Conn, error) {
	dialer := &net.Dialer{
		Timeout: DefaultTimeout,
	}
	nconn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := NewConn(ctx, nconn)
	return conn, nil
}

type Conn struct {
	Reader
	Writer

	// keeps a reference to the connection so TLS can be created in future.
	conn io.ReadWriteCloser
	// associated context will be used for cancellation in future.
	ctx context.Context
	// last caught error
	err error

	mu   sync.RWMutex
	once sync.Once
	wg   sync.WaitGroup

	stopped bool
	// helloAck becomes true after MRIM_CS_HELLO_ACK retrived.
	helloAck bool

	// TODO
	seq uint32

	// ping interval retrieved with MRIM_CS_HELLO_ACK.
	pingInterval time.Duration
	pingTimer    *time.Timer
}

func NewConn(ctx context.Context, conn io.ReadWriteCloser) *Conn {
	c := &Conn{
		ctx:  ctx,
		conn: conn,
	}
	c.Writer = Writer{
		bw: bufio.NewWriter(c.conn),
	}
	c.Reader = Reader{
		br:  bufio.NewReader(c.conn),
		buf: make([]byte, mraBufSize),
	}
	return c
}

func (c *Conn) Run(username, password string, status uint32, clientDesc string) {
	c.once.Do(func() {
		err := c.setupConnection(username, password, status, clientDesc)
		if err != nil {
			c.fatal(err)
		}

		c.mu.Lock()
		c.run()
		c.mu.Unlock()
	})
}

func (c *Conn) run() {
	if !c.helloAck {
		c.fatal(errNoHello)
	}

	go c.readLoop()

	if c.pingInterval > 0 {
		pingInterval := c.pingInterval
		if c.pingTimer == nil {
			c.pingTimer = time.AfterFunc(c.pingInterval, func() {
				c.ping(pingInterval)
			})
		} else {
			c.pingTimer.Reset(pingInterval)
		}
	}
}

func (c *Conn) setupConnection(username, password string, status uint32, clientDesc string) error {
	err := c.hello()
	if err != nil {
		return err
	}

	err = c.auth(username, password, status, clientDesc)
	if err != nil {
		return err
	}

	return nil
}

// FIXME(varankinv): Conn.Close
func (c *Conn) Close() (err error) {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
	}

	if c.pingTimer != nil {
		c.pingTimer.Stop()
		c.pingTimer = nil
	}

	// make sure we have flushed the outbound
	if c.conn != nil {
		if c.bw.Buffered() > 0 {
			err = c.Flush()
			if err != nil {
				debugf("failed to flush pending data: %v", err)
			}
		}
		err = c.conn.Close()
	}
	c.mu.Unlock()

	c.wg.Wait()

	return err
}

func (c *Conn) Do(ctx context.Context, msg uint32, data []byte) error {
	// TODO: acquire sequence
	c.mu.Lock()
	seq := c.seq
	c.seq++
	c.mu.Unlock()

	var p Packet
	p.Seq = seq
	p.Msg = msg
	p.Len = uint32(len(data))
	p.Data = data

	err := c.send(ctx, p)

	// TODO: release sequence

	return err
}

func (c *Conn) Send(ctx context.Context, p Packet) error {
	return c.send(ctx, p)
}

func (c *Conn) send(ctx context.Context, p Packet) (err error) {
	c.mu.RLock()
	stopped := c.stopped
	c.mu.RUnlock()

	if stopped {
		err = io.EOF
	} else {
		err = c.WritePacket(p)
	}

	if err != nil {
		debug(PacketError{p, fmt.Errorf("packet droped: %v", err)})
	} else {
		return c.Flush()
	}
	return
}

// hello sends "MRIM_CS_HELLO" message and reads the reply.
func (c *Conn) hello() (err error) {
	var p Packet
	p.Header.Msg = MsgCSHello

	err = c.send(c.ctx, p)
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err = c.ReadPacket()
	if err != nil {
		return err
	}

	if p.Msg != MsgCSHelloAck {
		return PacketError{p, errUnknownPacket}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.helloAck = true

	pingInterval := binary.LittleEndian.Uint32(p.Data)
	log.Printf("> received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", p.Seq, p.Msg, pingInterval)

	if pingInterval > 0 {
		c.pingInterval = time.Duration(pingInterval) * time.Second
	}

	return nil
}

type AuthError struct {
	s string
}

func (e AuthError) Error() string {
	return e.s
}

// auth sends "MRIM_CS_LOGIN2" and reads the reply.
func (c *Conn) auth(username, password string, status uint32, clientDesc string) (err error) {
	pw := PacketWriter{}
	pw.WriteData(username)
	pw.WriteData(password)
	pw.WriteData(status)
	pw.WriteData(clientDesc)
	for i := 0; i < 5; i++ {
		pw.WriteData(0) // internal fields
	}

	err = c.send(c.ctx, pw.Packet(MsgCSLogin2))
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err := c.ReadPacket()
	if err != nil {
		return err
	}

	switch p.Msg {
	case MsgCSLoginAck:
		log.Printf("> received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x\n", p.Seq, p.Msg)

	case MsgCSLoginRej:
		reason, err := unpackLPS(p.Data)
		if err != nil {
			return fmt.Errorf("mrim: cound not read auth rejection reason: %v", err)
		}
		log.Printf("> received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", p.Seq, p.Msg, reason)
		return AuthError{reason}

	default:
		return PacketError{p, errUnknownPacket}
	}

	return nil
}

func (c *Conn) ping(d time.Duration) {
	c.mu.RLock()
	if !c.helloAck {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	// NOTE: there is such thing as pong.
	var p Packet
	p.Header.Msg = MsgCSPing
	c.Send(c.ctx, p)

	c.pingTimer.Reset(d)
}

// readLoop is run in a goroutine, reading incoming packets.
func (c *Conn) readLoop() {
	c.wg.Add(1)
	defer c.wg.Done()

	var stopped bool

	for !stopped {
		p, err := c.ReadPacket()
		if err != nil {
			c.fatal(err)
			break
		}

		c.mu.RLock()
		stopped = c.stopped
		c.mu.RUnlock()

		// packets which (mostly all) are not replies
		switch p.Header.Msg {
		case MsgCSUserInfo:
			debugf("> received \"MRIM_CS_USER_INFO\" packet: %04x", p.Msg)

		case MrimCSOfflineMessageAck:
			// TODO(varankinv): send MRIM_CS_DELETE_OFFLINE_MESSAGE for each offline message.
			debugf("> received \"MRIM_CS_OFFLINE_MESSAGE_ACK\" packet: %04x", p.Msg)

		case MsgCSContactList2:
			debugf("> received \"MRIM_CS_CONTACT_LIST2\" packet: %04x", p.Msg)
		}
	}
}

func (c *Conn) fatal(err error) {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	debugf("fatal: %v", err)

	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()

	c.Close()
}

type Reader struct {
	br  *bufio.Reader
	buf []byte
}

func (r *Reader) ReadPacket() (p Packet, err error) {
	err = readPacketHeader(r.br, &p)
	if err != nil {
		return p, fmt.Errorf("mrim: cound not read packet header: %v", err)
	}

	n, err := r.br.Read(r.buf)
	if err != nil {
		return p, fmt.Errorf("mrim: cound not read packet body: %v", err)
	}
	if n < int(p.Len) {
		return p, fmt.Errorf("mrim: read less that expected: read %d, want %d", n, p.Len)
	}
	// TODO(varankinv): what those first n-len bytes for?
	p.Data = r.buf[n-int(p.Len): n]
	debugf("< received \"???\" packet: %d, %04x %d (%d) %v", p.Seq, p.Msg, p.Len, n, p.Data)
	return
}

type Writer struct {
	bw *bufio.Writer
}

func (w *Writer) WritePacket(p Packet) error {
	debugf("> sent \"???\" packet: %d, %04x %d %v", p.Seq, p.Msg, p.Len, p.Data)
	return writePacket(w.bw, p)
}

func (w *Writer) Flush() error {
	debugf("> flush: %d %d", w.bw.Buffered(), w.bw.Available())
	return w.bw.Flush()
}

func debug(v ...interface{}) {
	log.Println(v...)
}

func debugf(format string, v ...interface{}) {
	log.Printf(format+"\n", v...)
}

// unpackLPS unpacks LSP (long pascal string, size uint32 + str string) from v.
func unpackLPS(v []byte) (string, error) {
	if v == nil {
		return "", nil
	}
	l := binary.LittleEndian.Uint32(v)
	v = v[4:]
	if int(l) > len(v) {
		return "", errors.New("out of bound")
	}
	return string(v[:l]), nil
}
