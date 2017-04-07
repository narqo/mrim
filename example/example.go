package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"strconv"
	"time"

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
	mrconn, err := mrim.Dial(ctx, loginAddr.String())
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer mrconn.Close()

	pingInterval, err := mrconn.Hello()
	if err != nil {
		log.Fatal(err)
	}

	go doPing(mrconn, pingInterval)

	err = mrconn.Auth(username, password, mrim.StatusOnline, versionTxt)
	if err != nil {
		log.Fatal(err)
	}

	sendMsg(mrconn, "hello again")

	<-time.After(45 * time.Second)
}

func sendMsg(conn *mrim.Conn, msg string) error {
	err := conn.SendMessage("v.varankin@corp.mail.ru", []byte(msg), 0)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func doPing(conn *mrim.Conn, interval uint32) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		select {
		case <-ticker.C:
			log.Println("ping")
			err := conn.Ping()
			if err != nil {
				log.Printf("ping error: %v\n", err)
			}
		}
	}
}
