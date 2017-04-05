package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
	"strconv"

	"github.com/narqo/mrim"
)

const (
	hostPort   = "mrim.mail.ru:2042"
	versionTxt = "go mrim client 1.0"
)

const mraBufLen = 65536

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

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	seq += 1
	h := mrim.NewHeader(seq, mrim.MrimCsHello, 0)
	if err := enc.Encode(h); err != nil {
		log.Fatalf("could not encode buf: %v", err)
	}
	log.Printf("hello: send %d bytes: %b", buf.Len(), buf.Bytes())

	n, err := buf.WriteTo(conn)
	if err != nil {
		log.Fatalf("could not send hello: %v", err)
	}
	log.Printf("hello: send %d bytes", n)

	buf.Grow(mraBufLen)
	n, err = buf.ReadFrom(conn)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("read %d %v\n", n, buf.Bytes())
}
