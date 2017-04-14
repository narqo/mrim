package mrim

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

var (
	ErrNoHello = errors.New("no hello")
)

const (
	LangEn = "en"
	LangRu = "ru"
	LangUa = "ua"
)

var DefaultUserAgent = fmt.Sprintf(`client="%s" version="%s"`, "GoClient", "1.0")

const (
	DefaultInitTimeout = 30 * time.Second
	DefaultTimeout     = 15 * time.Minute
)

var defaultLogger Logger = log.New(os.Stderr, "mrim: ", log.Lshortfile)

type Logger interface {
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

type Options struct {
	Addr      string
	Username  string
	Password  string
	Status    uint32
	UserAgent string
	Lang      string // (>=1.16)
	Logger    Logger
}

type Client struct {
	conn   *Conn
	logger Logger

	loginAddr net.Addr

	userAgent string
	lang      string
	// helloAck becomes true after MRIM_CS_HELLO_ACK received.
	helloAck bool
}

func NewClient(ctx context.Context, opt *Options) (*Client, error) {
	c := &Client{
		userAgent: opt.UserAgent,
		lang:      opt.Lang,
	}

	if opt.UserAgent != "" {
		c.userAgent = opt.UserAgent
	} else {
		c.userAgent = DefaultUserAgent
	}

	if opt.Lang != "" {
		c.lang = opt.Lang
	} else {
		c.lang = LangRu
	}

	if opt.Logger != nil {
		c.logger = opt.Logger
	} else {
		c.logger = defaultLogger
	}

	err := c.Connect(ctx, opt.Addr, opt.Username, opt.Password, opt.Status)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) Connect(ctx context.Context, address, username, password string, status uint32) error {
	if c.conn != nil {
		return errors.New("mrim: already connected")
	}
	err := c.dial(ctx, address)
	if err != nil {
		return err
	}

	err = c.Hello(ctx)
	if err != nil {
		return err
	}

	err = c.Auth(ctx, username, password, status)
	if err != nil {
		return err
	}

	// after this point conn is meant to be established, run the conn reader
	c.conn.Run()

	return nil
}

func parseLoginAddr(data []byte) (net.Addr, error) {
	d := bytes.IndexByte(data, ':')
	if d == -1 {
		return nil, fmt.Errorf("bad login addr: %v", data)
	}
	host, data := data[:d], data[d+1:]
	port, err := strconv.ParseUint(string(data[:4]), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("bad login addr: %v", data)
	}

	ip := net.ParseIP(string(host))
	loginAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}
	return loginAddr, nil
}

func dial(ctx context.Context, addr string, timeout time.Duration) (*Conn, error) {
	dialer := &net.Dialer{
		Timeout: timeout,
	}
	nconn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := NewConn(ctx, nconn)
	return conn, nil
}

// dial initializes net connection to login host retrieved from server address.
// TODO(varankinv): refactor dial()
func (c *Client) dial(ctx context.Context, address string) error {
	nconn, err := net.DialTimeout("tcp", address, DefaultInitTimeout)
	if err != nil {
		return err
	}
	defer nconn.Close()

	data := make([]byte, 24)
	if _, err := nconn.Read(data); err != nil {
		return err
	}
	loginAddr, err := parseLoginAddr(data)
	if err != nil {
		return err
	}

	c.logger.Printf("loggin addr: %s\n", loginAddr.String())

	conn, err := dial(ctx, loginAddr.String(), DefaultTimeout)
	if err != nil {
		return fmt.Errorf("could not dial to login addr: %v", err)
	}

	select {
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	default:
	}

	c.loginAddr = loginAddr
	c.conn = conn

	return nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	if err != nil {
		c.conn = nil
	}
	return nil
}

// Hello sends "MRIM_CS_HELLO" message and reads the reply.
// It's an error to call Hello more than once.
func (c *Client) Hello(ctx context.Context) (err error) {
	if c.helloAck {
		return errors.New("mrim: repeative hello call")
	}
	return c.hello(ctx)
}

// hello is an idempotent version of Hello.
// It process the response and updates underlying conn's pingInterval according to reply.
func (c *Client) hello(ctx context.Context) (err error) {
	if c.helloAck {
		return nil
	}

	var p Packet
	p.Header.Msg = MsgCSHello

	err = c.conn.Send(ctx, p)
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err = c.conn.ReadPacket()
	if err != nil {
		return err
	}

	if p.Msg != MsgCSHelloAck {
		return PacketError{p, errUnknownPacket}
	}

	c.helloAck = true

	pingInterval := binary.LittleEndian.Uint32(p.Data)
	c.logger.Printf("> received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, ping %d\n", p.Seq, p.Msg, pingInterval)

	if pingInterval > 0 {
		// FIXME(varankinv): think of a better way of setting pingInterval.
		c.conn.mu.Lock()
		c.conn.pingInterval = time.Duration(pingInterval) * time.Second
		c.conn.mu.Unlock()
	}

	return nil
}

type AuthError struct {
	s string
}

func (e AuthError) Error() string {
	return e.s
}

// Auth sends "MRIM_CS_LOGIN2" and reads the reply.
func (c *Client) Auth(ctx context.Context, username, password string, status uint32) (err error) {
	if !c.helloAck {
		return ErrNoHello
	}

	pCsLogin2 := c.packetCsLogin2(ctx, username, password, status)
	err = c.conn.Send(ctx, pCsLogin2)
	if err != nil {
		return err
	}

	// read reply here because readLoop hasn't been started yet.
	p, err := c.conn.ReadPacket()
	if err != nil {
		return err
	}

	switch p.Msg {
	case MsgCSLoginAck:
		c.logger.Printf("> received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x\n", p.Seq, p.Msg)

	case MsgCSLoginRej:
		reason, err := unpackLPS(p.Data)
		if err != nil {
			return fmt.Errorf("mrim: cound not read auth rejection reason: %v", err)
		}
		c.logger.Printf("> received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, reason %q\n", p.Seq, p.Msg, reason)
		return AuthError{reason}

	default:
		return PacketError{p, errUnknownPacket}
	}

	return nil
}

func (c *Client) packetCsLogin2(ctx context.Context, username, password string, status uint32) Packet {
	pw := PacketWriter{}
	pw.WriteData(username)
	pw.WriteData(password)
	pw.WriteData(status)
	pw.WriteData(0) // spec_status_uri
	pw.WriteData(0) // status_title
	pw.WriteData(0) // status_desc
	pw.WriteData(0) // features
	pw.WriteData(c.userAgent)
	//pw.WriteData(c.lang)
	pw.WriteData([]byte{' '}) // client_desc
	return pw.Packet(MsgCSLogin2)
}

// Send sends packet p to the server.
func (c *Client) Send(ctx context.Context, p Packet) error {
	return c.conn.Send(ctx, p)
}

// Recv reads next packet from the server.
func (c *Client) Recv() (p Packet, err error) {
	if !c.helloAck {
		return p, ErrNoHello
	}
	return c.conn.Recv()
}
