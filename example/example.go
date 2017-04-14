package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/narqo/mrim"
)

const (
	hostPort = "mrim.mail.ru:2042"
)

var (
	username  = flag.String("u", "test-user-1991@mail.ru", "username")
	password  = flag.String("p", "", "password")
	recipient = flag.String("t", "", "recipient")
)

func main() {
	flag.Parse()

	errc := make(chan error, 1)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errc <- fmt.Errorf("%s", <-c)
	}()

	ctx := context.Background()
	//ctx, cancel := context.WithDeadline(ctx, time.Now().Add(15*time.Second))

	//time.AfterFunc(30*time.Second, func() {
	//	fmt.Println("cancel context")
	//	cancel()
	//})

	opt := &mrim.Options{
		Addr:      hostPort,
		Username:  *username,
		Password:  *password,
		UserAgent: mrim.DefaultUserAgent,
		Status:    mrim.StatusOnline,
	}
	c, err := mrim.NewClient(ctx, opt)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	go spamChat(ctx, c, *recipient)
	go readChat(ctx, c)

	fmt.Printf("exiting %v\n", <-errc)
}

func readChat(ctx context.Context, c *mrim.Client) {
	log.Println("read chat")
	for {
		p, err := c.Recv()
		if err != nil {
			if err == context.Canceled {
				log.Printf("context was canceld: %v\n", err)
				break
			}
			log.Printf("could not read reply: %v\n", err)
			continue
		}
		log.Printf("received packet: %d, %04x %v\n", p.Seq, p.Msg, p.Data)
	}
}

func spamChat(ctx context.Context, c *mrim.Client, to string) {
	log.Println("spam chat")
	for i := 0; i < 5; i++ {
		err := sendMsg(ctx, c, to, fmt.Sprintf("Поехали! Test message %d", i))
		if err != nil {
			log.Printf("could not send message: %v\n", err)
			continue
		}
		time.Sleep(3 * time.Second)
	}
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
