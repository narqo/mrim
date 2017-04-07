package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
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
	dialer := &net.Dialer{
		Timeout: 35 * time.Second,
	}
	nconn, err := dialer.DialContext(ctx, "tcp", loginAddr.String())
	if err != nil {
		log.Fatalf("could not dial to loginaddr: %v", err)
	}
	defer nconn.Close()

	// sequence
	var seq uint32

	err = sendHello(nconn, seq)
	if err != nil {
		log.Fatal(err)
	}

	seq += 1
	err = sendAuth(nconn, seq, username, password, mrim.StatusOnline)
	if err != nil {
		log.Fatal(err)
	}
}

func writeHeader(conn io.ReadWriter, seq, msg, dlen uint32) (err error) {
	var buf bytes.Buffer

	err = binary.Write(&buf, binary.LittleEndian, mrim.CSMagic)
	if err != nil {
		return err
	}
	err = binary.Write(&buf, binary.LittleEndian, mrim.ProtoVersion)
	if err != nil {
		return err
	}
	err = binary.Write(&buf, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Write(&buf, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Write(&buf, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}

	var from, fromPort uint32 // not used, must be zero
	err = binary.Write(&buf, binary.LittleEndian, from)
	if err != nil {
		return err
	}
	err = binary.Write(&buf, binary.LittleEndian, fromPort)
	if err != nil {
		return err
	}

	reserved := make([]byte, 16) // not used
	_, err = buf.Write(reserved)
	if err != nil {
		return err
	}

	_, err = buf.WriteTo(conn)
	if err != nil {
		return err
	}
	return nil
}

func readHeader(conn io.ReadWriter, seq, msg, dlen *uint32) (err error) {
	var magic, version uint32
	err = binary.Read(conn, binary.LittleEndian, &magic)
	if err != nil {
		return err
	}
	if magic != mrim.CSMagic {
		return fmt.Errorf("wrong magic: %08x", magic)
	}
	err = binary.Read(conn, binary.LittleEndian, &version)
	if err != nil {
		return err
	}
	//log.Printf("read head: version %d\n", version)
	err = binary.Read(conn, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Read(conn, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Read(conn, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}
	return nil
}

func readBody(conn io.ReadWriteCloser, dlen uint32) (err error) {
	// TODO(varankinv):
	buf := make([]byte, dlen + 65000)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	log.Printf("read body: %d bytes, dlen %d, %q\n", n, dlen, buf[:n])
	return nil
}

func sendHello(conn io.ReadWriteCloser, seq uint32) (err error) {
	err = writeHeader(conn, seq, mrim.MrimCSHello, 0)
	if err != nil {
		return err
	}

	var rxSeq, rxMsg, rxDlen uint32
	err = readHeader(conn, &rxSeq, &rxMsg, &rxDlen)
	if err != nil {
		return err
	}

	if rxMsg != mrim.MrimCSHelloAck {
		return fmt.Errorf("unknown packet received: %04x", rxMsg)
	}
	log.Printf("received \"MRIM_CS_HELLO_ACK\" packet: %d, %04x, %d\n", rxSeq, rxMsg, rxDlen)

	return readBody(conn, rxDlen)
}

type packet struct {
	buf bytes.Buffer
}

func (p *packet) WriteData(v ...interface{}) (err error) {
	for _, v := range v {
		err = packData(&p.buf, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *packet) WriteTo(w io.Writer) (int64, error) {
	return p.buf.WriteTo(w)
}

func sendAuth(conn io.ReadWriteCloser, seq uint32, username, password string, status uint32) (err error) {
	dlen := 4 + len(username) + 4 + len(password) + 4 + len(versionTxt) + 20 // 24 = 4 * 6 (online status (uint32) and 4 internal fields (uint32))
	err = writeHeader(conn, seq, mrim.MrimCSLogin2, uint32(dlen))
	if err != nil {
		return err
	}
	p := packet{}
	err = p.WriteData(username, password, status, versionTxt)
	if err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		err = p.WriteData(uint32(0)) // internal fields
		if err != nil {
			return err
		}
	}
	n, err := p.WriteTo(conn)
	if err != nil {
		return err
	}
	log.Printf("auth: send %d bytes\n", n)

	var rxSeq, rxMsg, rxDlen uint32
	err = readHeader(conn, &rxSeq, &rxMsg, &rxDlen)
	if err != nil {
		return err
	}

	err = readBody(conn, rxDlen)
	if err != nil {
		return err
	}

	switch rxMsg {
	case mrim.MrimCSLoginAck:
		log.Printf("received \"MRIM_CS_LOGIN_ACK\" packet: %d, %04x, %d\n", rxSeq, rxMsg, rxDlen)
		return nil
	case mrim.MrimCSLoginRej:
		log.Printf("received \"MRIM_CS_LOGIN_REJ\" packet: %d, %04x, %d\n", rxSeq, rxMsg, rxDlen)
		return nil
	}

	return fmt.Errorf("unknown packet received: %04x", rxMsg)
}

func packData(buf *bytes.Buffer, v interface{}) error {
	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case uint32:
		return binary.Write(buf, binary.LittleEndian, v)
	case int:
		return binary.Write(buf, binary.LittleEndian, uint32(v))
	case string:
		err := binary.Write(buf, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = buf.WriteString(v)
		if err != nil {
			return err
		}
	}
	return nil
}
