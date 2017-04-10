package mrim

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

const mraBufSize = 32768

var (
	errUnknownPacket = errors.New("unknown packet")
)

type Conn struct {
	Reader
	Writer

	// keeps a reference to the connection so TLS can be created in future.
	conn io.ReadWriteCloser
	// associated context will be used for cancellation in future.
	ctx context.Context
	// TODO(varankinv): last caught error
	err error

	mu   sync.RWMutex
	once sync.Once
	wg   sync.WaitGroup

	stopped bool
	// TODO(varankinv): seq pool
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

func (c *Conn) Run() {
	c.once.Do(func() {
		c.mu.Lock()
		c.run()
		c.mu.Unlock()
	})
}

func (c *Conn) run() {
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

	if err == nil {
		return c.Flush()
	} else {
		debug(PacketError{p, fmt.Errorf("packet droped: %v", err)})
	}
	return
}

func (c *Conn) ping(d time.Duration) {
	c.mu.RLock()
	if c.stopped {
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
	// reusable buffer for header parsing
	hbuf [20]byte
	buf  []byte
}

func (r *Reader) ReadPacket() (p Packet, err error) {
	buf := r.hbuf[:]
	_, err = io.ReadFull(r.br, buf)
	if err != nil {
		return p, fmt.Errorf("mrim: cound not read packet header: %v", err)
	}
	err = readPacketHeader(buf, &p)
	if err != nil {
		return p, fmt.Errorf("mrim: cound not parse packet header: %v", err)
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
