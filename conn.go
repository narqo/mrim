package mrim

import (
	"bufio"
	"bytes"
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
	flushChanSize = 1024

	DefaultTimeout = 25 * time.Second
)

var (
	errNoHello = errors.New("no hello")
	errPacketDropped = errors.New("packet droped")
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
	if tconn, ok := nconn.(*net.TCPConn); ok {
		err := tconn.SetKeepAlive(true)
		if err != nil {
			nconn.Close()
			return nil, err
		}
	}
	conn := NewConn(ctx, nconn)
	return conn, nil
}

/*
func connectLoginAddr(ctx context.Context, loginAddr net.Addr) error {
	dialer := &net.Dialer{
		Timeout: DefaultTimeout,
	}
	conn, err := dialer.DialContext(ctx, "tcp", loginAddr.String())
	if err != nil {
		return err
	}

	c := NewConn(ctx, conn)
	c.Run()

	return nil
}
*/

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
	wg   *sync.WaitGroup

	// flusher internal channel
	fch chan struct{}
	// stopper used to stop flusher
	stopper chan struct{}

	stopped bool
	// helloAck becomes true after MRIM_CS_HELLO_ACK retrived.
	helloAck bool

	// ping interval retrieved with MRIM_CS_HELLO_ACK.
	pingInterval time.Duration
	pingTimer    *time.Timer
}

func NewConn(ctx context.Context, conn io.ReadWriteCloser) *Conn {
	c := &Conn{
		ctx:     ctx,
		conn:    conn,
		fch:     make(chan struct{}, flushChanSize),
		stopper: make(chan struct{}),
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
	go c.flusher()

	if c.pingInterval > 0 {
		if c.pingTimer == nil {
			c.pingTimer = time.AfterFunc(c.pingInterval, func() {
				c.ping(c.pingInterval)
			})
		} else {
			c.pingTimer.Reset(c.pingInterval)
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
func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Do(ctx context.Context, msg uint32, data []byte) error {
	// TODO: acquire sequence
	seq := uint32(0)

	var p Packet
	p.Seq = seq
	p.Msg = msg
	p.Len = uint32(len(data))
	p.Data = data

	err := c.sendFlush(ctx, p)

	// TODO: release sequence

	return err
}

func (c *Conn) Send(ctx context.Context, p Packet) error {
	return c.sendFlush(ctx, p)
}

func (c *Conn) sendFlush(ctx context.Context, p Packet) (err error) {
	err = c.send(ctx, p)
	if err != nil {
		return
	}
	c.kickFlusher()
	return
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
		debugf("%v", PacketError{p, errPacketDropped})
	}
	return
}

// hello sends "MRIM_CS_HELLO" message and reads the reply.
func (c *Conn) hello() (err error) {
	// send packet doing manual Flush, because flusher hasn't been started yet.
	var p Packet
	p.Header.Msg = mrimCSHello

	err = c.send(c.ctx, p)
	if err != nil {
		return err
	}
	err = c.Flush()
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err = c.ReadPacket()
	if err != nil {
		return err
	}

	if p.Msg != mrimCSHelloAck {
		return PacketError{p, errUnknownPacket}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.helloAck = true

	pingInterval := binary.LittleEndian.Uint32(p.Data)
	log.Printf("received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", p.Seq, p.Msg, pingInterval)

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
func (c *Conn) auth(username, password string, status uint32, clientDesc string) error {
	// FIXME: pack data into []byte
	var data bytes.Buffer
	writeData(&data, username)
	writeData(&data, password)
	writeData(&data, status)
	writeData(&data, clientDesc)
	for i := 0; i < 5; i++ {
		writeData(&data, uint32(0)) // internal fields
	}

	err := c.Do(c.ctx, mrimCSLogin2, data.Bytes())
	if err != nil {
		return err
	}
	// flush manually because flusher hasn't been started yet
	err = c.Flush()
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err := c.ReadPacket()
	if err != nil {
		return err
	}

	switch p.Msg {
	case mrimCSLoginAck:
		log.Printf("received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x\n", p.Seq, p.Msg)

	case mrimCSLoginRej:
		reason, err := unpackLPS(p.Data)
		if err != nil {
			return fmt.Errorf("mrim: cound not read auth rejection reason: %v", err)
		}
		log.Printf("received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", p.Seq, p.Msg, reason)
		return AuthError{reason}

	default:
		return PacketError{p, errUnknownPacket}
	}

	return nil
}

func (c *Conn) ping(d time.Duration) {
	c.mu.Lock()
	if !c.helloAck {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	// NOTE: there is such thing as pong.
	var p Packet
	p.Header.Msg = mrimCSPing
	c.Send(c.ctx, p)

	c.pingTimer.Reset(d)
}

// readLoop is run in a goroutine, reading incoming packets.
func (c *Conn) readLoop() {
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
		case mrimCSUserInfo:
			debugf("received \"MRIM_CS_USER_INFO\" packet: %04x", p.Msg)

		case mrimCSOfflineMessageAck:
			// TODO(varankinv): send MRIM_CS_DELETE_OFFLINE_MESSAGE for each offline message.
			debugf("received \"MRIM_CS_OFFLINE_MESSAGE_ACK\" packet: %04x", p.Msg)

		case mrimCSContactList2:
			debugf("received \"MRIM_CS_CONTACT_LIST2\" packet: %04x", p.Msg)
		}
	}
}

// flusher is run in a goroutine, processing flush requests for the Writer.
func (c *Conn) flusher() {
	c.mu.RLock()
	w := c.Writer
	helloAck := c.helloAck
	c.mu.RUnlock()

	if !helloAck {
		debugf("flush without hello")
		return
	}

	var stopped bool

	for !stopped {
		select {
		case <-c.fch:
			c.mu.RLock()
			if w.bw.Buffered() > 0 {
				c.mu.RUnlock()

				err := w.Flush()
				if err != nil {
					debugf("failed to flush writter")
				}
				break
			}
			c.mu.RUnlock()

		case <-c.stopper:
			stopped = true
		}
	}
}

// kickFlusher send a signal to fch to trigger packets flush in the flusher.
func (c *Conn) kickFlusher() {
	if c.bw != nil {
		select {
		case c.fch <- struct{}{}:
		default:
		}
	}
}

func (c *Conn) stopFlusher() {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		close(c.stopper)
	}
	c.mu.Unlock()
}

func (c *Conn) fatal(err error) {
	c.mu.Lock()
	debugf("fatal: %v", err)
	c.err = err
	c.mu.Unlock()

	c.stopFlusher()
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
		return p, fmt.Errorf("read less that expected: read %d, want %d", n, p.Len)
	}
	// TODO(varankinv): what those first n-len bytes for?
	p.Data = r.buf[n-int(p.Len): n]
	debugf("< received \"???\" packet: %d, %04x %d %v", p.Seq, p.Msg, p.Len, p.Data)
	return
}

func writeData(w io.Writer, v interface{}) error {
	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case int:
		return binary.Write(w, binary.LittleEndian, uint32(v))
	case uint:
		return binary.Write(w, binary.LittleEndian, uint32(v))
	case uint32:
		return binary.Write(w, binary.LittleEndian, v)
	case string:
		err := binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(v))
		if err != nil {
			return err
		}
	case []byte:
		err := binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = w.Write(v)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	return nil
}

type Writer struct {
	bw *bufio.Writer
}

func (w *Writer) WritePacket(p Packet) error {
	debugf("> sent \"???\" packet: %d, %04x %d %v", p.Seq, p.Msg, p.Len, p.Data)
	return writePacket(w.bw, p)
}

func (w *Writer) Flush() error {
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
