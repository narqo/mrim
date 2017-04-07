package main

import (
	"bytes"
	"context"
	"flag"
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
	username = flag.String("u", "test-user-1991@mail.ru", "username")
	password = flag.String("p", "", "password")
)

const recipient = "test-user-1991@mail.ru"

func main() {
	flag.Parse()

	ctx := context.Background()

	loginAddr, err := initLoginAddr(hostPort)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("login host port: %s %d\n", loginAddr.IP, loginAddr.Port)

	mrconn, err := mrim.Dial(ctx, loginAddr.String())
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer mrconn.Close()

	pingInterval, err := mrconn.Hello()
	if err != nil {
		log.Fatal(err)
	}

	if pingInterval > 0 {
		go doPing(mrconn, pingInterval)
	}

	err = mrconn.Auth(*username, *password, mrim.StatusOnline, versionTxt)
	if err != nil {
		log.Fatal(err)
	}

	sendMsg(mrconn, "hello!")
}

func initLoginAddr(hostPort string) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data := make([]byte, 24)
	if _, err := conn.Read(data); err != nil {
		return nil, err
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

	ip := net.ParseIP(string(host))
	loginAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}
	return loginAddr, nil
}

func sendMsg(conn *mrim.Conn, msg string) error {
	err := conn.SendMessage(recipient, []byte(msg), mrim.MessageFlagNorecv)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

// TODO: use time.AfterFunc instead of for-loop
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
