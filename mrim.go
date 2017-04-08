// +build ignore

package example

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
	"time"
)

type Conn struct {
	Reader
	Writer
	conn     io.ReadWriteCloser
	helloAck bool
}

func Dial(ctx context.Context, addr string) (*Conn, error) {
	dialer := &net.Dialer{
		Timeout: 25 * time.Second,
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
	conn := NewConn(nconn)
	return conn, nil
}

const mraBufLen = 65536

func NewConn(conn io.ReadWriteCloser) *Conn {
	var b [mraBufLen]byte
	return &Conn{
		Reader: Reader{R: bufio.NewReader(conn), buf: b[:]},
		Writer: Writer{W: bufio.NewWriter(conn)},
		conn:   conn,
	}
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Do(ctx context.Context, msg uint32, data []byte) error {



}

func (c *Conn) doSend(ctx context.Context, p Packet) error {
	err := c.WritePacket(p)
	if err != nil {
		return err
	}
	return c.Flush()
}

func (c *Conn) Hello() (uint32, error) {
	if c.helloAck {
		return 0, errors.New("repetitive hello call")
	}
	return c.hello()
}

func (c *Conn) hello() (pingInterval uint32, err error) {
	if c.helloAck {
		return 0, nil
	}

	err = c.Do(context.TODO(), mrimCSHello, nil)
	if err != nil {
		return 0, err
	}

	p, err := c.ReadPacket()
	if err != nil {
		return 0, err
	}

	if p.Msg != mrimCSHelloAck {
		return 0, fmt.Errorf("unknown packet received: %04x", p.Msg)
	}

	c.helloAck = true

	pingInterval = binary.LittleEndian.Uint32(p.Data)
	log.Printf("received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", p.Seq, p.Msg, pingInterval)

	return
}

type AuthError struct {
	s string
}

func (e AuthError) Error() string {
	return e.s
}

func (c *Conn) Auth(username, password string, status uint32, clientDesc string) error {
	if _, err := c.hello(); err != nil {
		return err
	}

	var data bytes.Buffer
	writeData(&data, username)
	writeData(&data, password)
	writeData(&data, status)
	writeData(&data, clientDesc)
	for i := 0; i < 5; i++ {
		writeData(&data, uint32(0)) // internal fields
	}
	err := c.Do(context.TODO(), mrimCSLogin2, data.Bytes())
	if err != nil {
		return err
	}

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
			return fmt.Errorf("cound not read auth rejection reason: %v", err)
		}
		log.Printf("received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", p.Seq, p.Msg, reason)
		return AuthError{reason}
	default:
		return fmt.Errorf("unknown packet received: %04x", p.Msg)
	}

	for {
		p, err = c.ReadPacket()
		if err != nil {
			return err
		}
		if p.Msg == mrimCSContactList2 {
			log.Printf("received \"MRIM_CS_CONTACT_LIST2\" packet: %04x\n", p.Msg)
			break
		}
	}

	return nil
}

func (c *Conn) Ping() error {
	if _, err := c.hello(); err != nil {
		return err
	}
	return c.Do(context.TODO(), mrimCSPing, nil)
}

func (c *Conn) SendMessage(to string, msg []byte, flags uint32) error {
	if !c.helloAck {
		return errors.New("no hello")
	}

	log.Printf("doSend message: %q, %q\n", to, msg)

	var data bytes.Buffer
	writeData(&data, flags)
	writeData(&data, to)
	writeData(&data, msg)
	writeData(&data, []byte{' '}) // message_rtf
	return c.Do(context.TODO(), mrimCSMessage, data.Bytes())
}

type Reader struct {
	R *bufio.Reader

	buf []byte
}

func (b *Reader) ReadPacket() (p Packet, err error) {
	err = readPacketHeader(b.R, &p)
	if err != nil {
		return p, fmt.Errorf("cound not read packet header: %v", err)
	}

	n, err := b.R.Read(b.buf)
	if err != nil {
		return p, fmt.Errorf("cound not read packet body: %v", err)
	}
	if n < int(p.Len) {
		return p, fmt.Errorf("read less that expected: read %d, want %d", n, p.Len)
	}
	// TODO(varankinv): what those first n-dlen bytes for?
	p.Data = b.buf[n-int(p.Len): n]
	log.Printf("received \"???\" packet: %d, %04x %d %q\n", p.Seq, p.Msg, p.Len, p.Data)
	return
}

type Writer struct {
	W *bufio.Writer
}

func (b *Writer) WritePacket(p Packet) error {
	return writePacket(b.W, p)
}

func (b *Writer) Flush() error {
	log.Printf("writer flush: %d\n", b.W.Buffered())
	return b.W.Flush()
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
