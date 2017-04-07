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

func (c *Conn) Hello() (uint32, error) {
	if c.helloAck {
		return 0, errors.New("repetitive hello call")
	}
	return c.hello(0)
}

func (c *Conn) hello(seq uint32) (pingInterval uint32, err error) {
	if c.helloAck {
		return 0, nil
	}

	err = c.WriteHeader(seq, mrimCSHello, 0)
	if err != nil {
		return 0, err
	}
	err = c.Flush()
	if err != nil {
		return 0, err
	}

	msg, body, err := c.readPacket()
	if err != nil {
		return 0, err
	}

	if msg != mrimCSHelloAck {
		return 0, fmt.Errorf("unknown packet received: %04x", msg)
	}

	c.helloAck = true

	pingInterval = binary.LittleEndian.Uint32(body)
	log.Printf("received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", seq, msg, pingInterval)

	return
}

type AuthError string

func (e AuthError) Error() string {
	return string(e)
}

func (c *Conn) Auth(username, password string, status uint32, clientDesc string) error {
	var seq uint32
	if _, err := c.hello(seq); err != nil {
		return err
	}
	seq++
	return c.auth(seq, username, password, status, clientDesc)
}

func (c *Conn) auth(seq uint32, username, password string, status uint32, clientDesc string) error {
	// 4 + len(str) is for LPSSIZE
	// 24 = 4 * 6 (online status (uint32) and 5 internal fields (uint32))
	dlen := 4 + len(username) + 4 + len(password) + 4 + len(clientDesc) + 24

	err := c.WriteHeader(seq, mrimCSLogin2, uint32(dlen))
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

	msg, body, err := c.readPacket()
	if err != nil {
		return err
	}

	switch msg {
	case mrimCSLoginAck:
		log.Printf("received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x\n", seq, msg)
	case mrimCSLoginRej:
		reason, err := unpackLPS(body)
		if err != nil {
			return fmt.Errorf("cound not read auth rejection reason: %v", err)
		}
		log.Printf("received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", seq, msg, reason)
		return AuthError(reason)
	default:
		return fmt.Errorf("unknown packet received: %04x", msg)
	}

	for {
		msg, _, err := c.readPacket()
		if err != nil {
			return err
		}
		if msg == mrimCSContactList2 {
			log.Printf("received \"MRIM_CS_CONTACT_LIST2\" packet: %04x\n", msg)
			break
		}
	}

	return nil
}

func (c *Conn) Ping() error {
	var seq uint32
	if _, err := c.hello(seq); err != nil {
		return err
	}
	seq++
	err := c.WriteHeader(seq, mrimCSPing, 0)
	if err != nil {
		return err
	}
	return c.Flush()
}

func (c *Conn) SendMessage(to string, msg []byte, flags uint32) error {
	if !c.helloAck {
		return errors.New("no hello")
	}
	return c.sendMessage(42, to, msg, flags)
}

func (c *Conn) sendMessage(seq uint32, to string, message []byte, flags uint32) error {
	log.Printf("send message: %d, %q, %q\n", seq, to, message)

	// 4 + len(str) for LPSSIZE(to)
	// 4 + len(str) for LPSSIZE(message)
	// 4 + len(" ") for LPSSIZE(msg_rtf)
	// 4 for flags uint32
	messageRTF := []byte{' '}
	dlen := 4 + len(to) + 4 + len(message) + 4 + len(messageRTF) + 4 + 4
	err := c.WriteHeader(seq, mrimCSMessage, uint32(dlen))
	if err != nil {
		return err
	}
	err = c.WriteData(flags, to, message, messageRTF)
	if err != nil {
		return err
	}
	return c.Flush()
}

func (c *Conn) readPacket() (msg uint32, body []byte, err error) {
	var seq uint32
	seq, msg, err = c.ReadHeader()
	if err != nil {
		return
	}
	body, err = c.ReadBody()
	if err != nil {
		return
	}
	log.Printf("received \"???\" packet: %d, %04x %q\n", seq, msg, body)
	return
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
	log.Printf("read body: %d bytes, dlen %d\n", n, p.dlen)
	p.dlen = 0
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
	log.Printf("writer flush: %d\n", p.W.Buffered())
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
		_, err = w.WriteString(v)
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
