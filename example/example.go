package main

import (
	"bytes"
	"context"
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
	password = "123"
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

	ctx := context.Background()

	ip := net.ParseIP(string(host))
	loginAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}
	mconn, err := mrim.Dial(ctx, loginAddr.String())
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer mconn.Close()

	err = mconn.Hello()
	if err != nil {
		log.Fatal(err)
	}
	err = mconn.Auth(username, password, mrim.StatusOnline, versionTxt)
	if err != nil {
		log.Fatal(err)
	}
}
