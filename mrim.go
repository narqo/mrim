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
	"time"
)

type Conn struct {
	Reader
	Writer
	conn        io.ReadWriteCloser
	helloAck    bool
	pingTimeout uint32
}

func Dial(ctx context.Context, addr string) (*Conn, error) {
	dialer := &net.Dialer{
		Timeout: 35 * time.Second,
	}
	nconn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := NewConn(nconn)
	return conn, nil
}

const mraBufLen = 65536

func NewConn(conn io.ReadWriteCloser) *Conn {
	return &Conn{
		Reader: Reader{R: bufio.NewReader(conn), buf: make([]byte, mraBufLen)},
		Writer: Writer{W: bufio.NewWriter(conn)},
		conn:   conn,
	}
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Hello() error {
	if c.helloAck {
		return errors.New("repetitive hello call")
	}
	return c.hello(0)
}

func (c *Conn) hello(seq uint32) error {
	err := c.WriteHeader(seq, MrimCSHello, 0)
	if err != nil {
		return err
	}
	err = c.Flush()
	if err != nil {
		return err
	}

	seq, msg, err := c.ReadHeader()
	if err != nil {
		return err
	}
	if msg != MrimCSHelloAck {
		return fmt.Errorf("unknown packet received: %04x", msg)
	}

	body, err := c.ReadBody()
	if err != nil {
		return err
	}
	c.helloAck = true
	c.pingTimeout = binary.LittleEndian.Uint32(body)
	log.Printf("received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", seq, msg, c.pingTimeout)

	return nil
}

type AuthError string

func (e AuthError) Error() string {
	return string(e)
}

func (c *Conn) Auth(username, password string, status uint32, clientDesc string) error {
	return c.auth(0, username, password, status, clientDesc)
}

func (c *Conn) auth(seq uint32, username, password string, status uint32, clientDesc string) error {
	// 4 + len(str) is for LPSSIZE
	// 24 = 4 * 6 (online status (uint32) and 5 internal fields (uint32))
	dlen := 4 + len(username) + 4 + len(password) + 4 + len(clientDesc) + 24

	err := c.WriteHeader(seq, MrimCSLogin2, uint32(dlen))
	if err != nil {
		return err
	}
	err = c.WriteData(username, password, status, clientDesc)
	for i := 0; i < 5; i++ {
		err = c.WriteData(uint32(0)) // internal fields
		if err != nil {
			return err
		}
	}
	err = c.Flush()
	if err != nil {
		return err
	}

	seq, msg, err := c.ReadHeader()
	if err != nil {
		return err
	}

	body, err := c.ReadBody()
	if err != nil {
		return err
	}

	switch msg {
	case MrimCSLoginAck:
		log.Printf("received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x\n", seq, msg)
		return nil
	case MrimCSLoginRej:
		reason, err := unpackLPS(body)
		if err != nil {
			return fmt.Errorf("cound not read auth rejection reason: %v", err)
		}
		log.Printf("received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", seq, msg, reason)
		return AuthError(reason)
	default:
		return fmt.Errorf("unknown packet received: %04x", msg)
	}
	return nil
}

func (c *Conn) Ping() error {
	err := c.WriteHeader(0, MrimCSPing, 0)
	if err != nil {
		return err
	}
	_, err = c.ReadBody()
	if err != nil {
		return err
	}
	return err
}

type Reader struct {
	R *bufio.Reader

	dlen uint32
	buf  []byte
}

func (p *Reader) ReadHeader() (seq, msg uint32, err error) {
	err = readHeader(p.R, &seq, &msg, &p.dlen)
	if err != nil {
		err = fmt.Errorf("cound not read header: %v", err)
	}
	return
}

func (p *Reader) ReadBody() ([]byte, error) {
	n, err := p.R.Read(p.buf)
	if err != nil {
		return nil, fmt.Errorf("cound not read body: %v", err)
	}
	if n < int(p.dlen) {
		return nil, fmt.Errorf("read less that expected: read %d, want %d", n, p.dlen)
	}
	// TODO(varankinv): what those first n-dlen bytes for?
	body := p.buf[n-int(p.dlen): n]
	log.Printf("read body: %d bytes, dlen %d, %b\n", n, p.dlen, body)
	return body, nil
}

func readHeader(r io.Reader, seq, msg, dlen *uint32) (err error) {
	var magic, version uint32
	err = binary.Read(r, binary.LittleEndian, &magic)
	if err != nil {
		return err
	}
	if magic != CSMagic {
		return fmt.Errorf("wrong magic: %08x", magic)
	}
	err = binary.Read(r, binary.LittleEndian, &version)
	if err != nil {
		return err
	}
	//log.Printf("read head: version %d\n", version)
	err = binary.Read(r, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}
	return nil
}

type Writer struct {
	W *bufio.Writer
}

func (p *Writer) WriteHeader(seq, msg, dlen uint32) error {
	return writeHeader(p.W, seq, msg, dlen)
}

func (p *Writer) WriteData(v ...interface{}) (err error) {
	for _, v := range v {
		err = packData(p.W, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Writer) Flush() error {
	return p.W.Flush()
}

var headerReserved = make([]byte, 16) // not used, must be filled with zeroes

func writeHeader(w io.Writer, seq, msg, dlen uint32) (err error) {
	err = binary.Write(w, binary.LittleEndian, CSMagic)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, ProtoVersion)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}

	var from, fromPort uint32 // not used, must be zero
	err = binary.Write(w, binary.LittleEndian, from)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, fromPort)
	if err != nil {
		return err
	}

	_, err = w.Write(headerReserved)
	return err
}

func packData(w *bufio.Writer, v interface{}) error {
	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case uint32:
		return binary.Write(w, binary.LittleEndian, v)
	case int:
		return binary.Write(w, binary.LittleEndian, uint32(v))
	case string:
		err := binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = w.WriteString(v)
		if err != nil {
			return err
		}
	}
	return nil
}

// unpackLPS unpacks LSP (long pascal string, size uint32 + str string).
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
