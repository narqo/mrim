package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/narqo/mrim"
)

const (
	hostPort   = "mrim.mail.ru:2042"
	versionTxt = "go mrim client 1.0"
)

var (
	username  = flag.String("u", "test-user-1991@mail.ru", "username")
	password  = flag.String("p", "", "password")
	recipient = flag.String("t", "", "recipient")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	opt := &mrim.Options{
		Addr:       hostPort,
		ClientDesc: versionTxt,
		Username:   *username,
		Password:   *password,
		Status:     mrim.StatusOnline,
	}
	c, err := mrim.NewClient(ctx, opt)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = spamChat(ctx, c, *recipient)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("exiting...")
}

func spamChat(ctx context.Context, c *mrim.Client, to string) (err error) {
	for i := 0; i < 10; i++ {
		err = sendMsg(ctx, c, to, fmt.Sprintf("test message %d", i))
		if err != nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	log.Printf("spam chat done: %v\n", err)
	return
}

func sendMsg(ctx context.Context, c *mrim.Client, to, msg string) error {
	msgRTF := []byte{' '}

	var p mrim.PacketWriter
	p.WriteData(mrim.MessageFlagNorecv) // flags
	p.WriteData(to)                     // to
	p.WriteData(msg)                    // message
	p.WriteData(msgRTF)                 // rtf message

	err := c.Send(ctx, p.Packet(mrim.MsgCSMessage))
	if err != nil {
		log.Fatal(err)
	}
	return nil
}
