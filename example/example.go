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

	mrconn, err := mrim.Dial(ctx, loginAddr.String())
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer mrconn.Close()

	mrconn.Run(*username, *password, mrim.StatusOnline, versionTxt)

	sendMsg(ctx, mrconn, "hello!")
	<-time.After(3 * time.Second)
	sendMsg(ctx, mrconn, "hey! busy?")
	<-time.After(3 * time.Second)
	sendMsg(ctx, mrconn, "sorry, me again")

	<-time.After(45 * time.Second)

	sendMsg(ctx, mrconn, "human! you're ignoring me!!111")
}

func initLoginAddr(hostPort string) (net.Addr, error) {
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
	log.Printf("login host port: %s %d\n", loginAddr.IP, loginAddr.Port)
	return loginAddr, nil
}

func sendMsg(ctx context.Context, conn *mrim.Conn, msg string) error {
	msgRTF := []byte{' '}

	var p mrim.PacketWriter
	p.WriteData(mrim.MessageFlagNorecv) // flags
	p.WriteData(recipient)   // to
	p.WriteData(msg)         // message
	p.WriteData(msgRTF)      // rtf message

	err := conn.Send(ctx, p.Packet(mrim.MsgCSMessage))
	if err != nil {
		log.Fatal(err)
	}
	return nil
}
