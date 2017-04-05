package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"

	"github.com/narqo/mrim"
)

const (
	hostPort   = "mrim.mail.ru:2042"
	versionTxt = "go mrim client 1.0"
)

var (
	username = "v.varankin@corp.mail.ru"
	password = ""
)

func main() {
	addr, err := net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}

	data := make([]byte, 24)
	if _, err := conn.Read(data); err != nil {
		log.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		log.Fatal(err)
	}

	d := bytes.IndexByte(data, ':')
	if d == -1 {
		log.Fatalf("bad login addr: %v", data)
	}
	host, data := data[:d], data[d+1:]
	port, err := strconv.ParseUint(string(data[:4]), 10, 16)
	if err != nil {
		log.Fatalf("bad login addr: %v", data)
	}

	log.Printf("login host port: %s %d\n", host, port)

	ip := net.ParseIP(string(host))
	loginAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}
	conn, err = net.DialTCP("tcp", nil, loginAddr)
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer conn.Close()

	// sequence
	var seq uint32
	var b bytes.Buffer

	err = sendHello(conn, seq, &b)
	if err != nil {
		log.Fatal(err)
	}
	seq += 1

	err = sendAuth(conn, seq, username, password, mrim.StatusOnline, &b)
	if err != nil {
		log.Fatal(err)
	}
	seq += 1
}

// TODO(varankinv): read packet with bufio reader
func readConn(conn io.ReadWriter) (msg uint32, data []byte, err error) {
	b := make([]byte, 44)
	m, err := io.ReadFull(conn, b)
	if err != nil {
		return
	}
	log.Printf("read %d %v\n", m, b)

	var rxSeq uint32
	rxSeq, msg, data, err = mrim.Unpack(b)
	if err != nil {
		return 0, nil, fmt.Errorf("could not unpack rx: %v", err)
	}
	log.Printf("unpack %d %04x %v\n", rxSeq, msg, data)
	return
}

func sendHello(conn io.ReadWriter, seq uint32, b *bytes.Buffer) error {
	err := mrim.Pack(b, seq, mrim.MrimCSHello, 0)
	if err != nil {
		return err
	}
	log.Printf("hello: sending %d %v\n", b.Len(), b.Bytes())

	n, err := b.WriteTo(conn)
	if err != nil {
		return fmt.Errorf("could not send hello: %v", err)
	}
	log.Printf("hello: send %d bytes\n", n)

	msg, _, err := readConn(conn)
	if err != nil {
		return err
	}

	if msg == mrim.MrimCSHelloAck {
		log.Println("received \"MRIM_CS_HELLO_ACK\" packet")
		return nil
	}
	return fmt.Errorf("unknown packet received: %04x", msg)
}

func sendAuth(conn io.ReadWriter, seq uint32, username, password string, status uint32, b *bytes.Buffer) error {
	dlen := len(username) + len(password) + len(versionTxt) + 24 // 24 = 4 * 6 (online status (uint32) and 5 dw (uint32))
	err := mrim.Pack(b, seq, mrim.MrimCSLogin2, uint32(dlen), username, password, status, versionTxt, 0, 0, 0, 0, 0)
	if err != nil {
		return err
	}
	log.Printf("auth: sending %d %v\n", b.Len(), b.Bytes())

	n, err := b.WriteTo(conn)
	if err != nil {
		return fmt.Errorf("could not send auth: %v", err)
	}
	log.Printf("auth: send %d bytes\n", n)

	msg, _, err := readConn(conn)
	if err != nil {
		return err
	}

	switch msg {
	case mrim.MrimCSLoginAck:
		log.Println("received \"MRIM_CS_LOGIN_ACK\" packet")
		return nil
	case mrim.MrimCSLoginRej:
		log.Println("received \"MRIM_CS_LOGIN_REJ\" packet")
		return nil
	}

	return fmt.Errorf("unknown packet received: %04x", msg)
}
