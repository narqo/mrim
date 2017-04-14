package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/narqo/mrim"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
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

		handlePacket(ctx, p)
	}
}

func handlePacket(ctx context.Context, p mrim.Packet) {
	switch p.Msg {
	case mrim.MsgCSMessageAck:
		msgID := binary.LittleEndian.Uint32(p.Data[0:])
		flags := binary.LittleEndian.Uint32(p.Data[4:])
		fromLen := binary.LittleEndian.Uint32(p.Data[8:])
		from := p.Data[12:fromLen]
		//msgLen := binary.LittleEndian.Uint32(p.Data[12+fromLen:])
		rawMsg := p.Data[16+fromLen:]
		log.Printf("received \"MRIM_CS_MESSAGE_ACK\" packet: %d, %04x %v %v %v %s\n", p.Seq, p.Msg, msgID, flags, from, rawMsg)

	case mrim.MsgCSMessageRecv:
		log.Printf("received \"MRIM_CS_MESSAGE_RECV\" packet: %d, %04x %v\n", p.Seq, p.Msg, p.Data)

	case mrim.MrimCSOfflineMessageAck:
		log.Printf("received \"MRIM_CS_OFFLINE_MESSAGE_ACK\" packet: %d, %04x %v\n", p.Seq, p.Msg, p.Data)

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

	m, _, err := transform.Bytes(charmap.Windows1251.NewEncoder(), []byte(msg))
	if err != nil {
		return err
	}

	var p mrim.PacketWriter
	p.WriteData(mrim.MessageFlagNorecv) // flags
	p.WriteData(to)                     // to
	p.WriteData(m)                      // message
	p.WriteData(msgRTF)                 // rtf message

	err = c.Send(ctx, p.Packet(mrim.MsgCSMessage))
	if err != nil {
		log.Fatal(err)
	}
	return nil
}
