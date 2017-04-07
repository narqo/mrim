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

func sendHello(conn io.ReadWriteCloser, seq uint32) (err error) {
	packet := mraWriter{
		wr: conn,
	}
	err = packet.WriteHeader(seq, mrim.MrimCSHello, 0)
	if err != nil {
		return err
	}
	packet.Flush()

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

func sendAuth(conn io.ReadWriteCloser, seq uint32, username, password string, status uint32) (err error) {
	packet := mraWriter{
		wr: conn,
	}
	dlen := 4 + len(username) + 4 + len(password) + 4 + len(versionTxt) + 20 // 24 = 4 * 6 (online status (uint32) and 4 internal fields (uint32))
	err = packet.WriteHeader(seq, mrim.MrimCSLogin2, uint32(dlen))
	if err != nil {
		return err
	}
	err = packet.WriteData(username, password, status, versionTxt)
	if err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		err = packet.WriteData(uint32(0)) // internal fields
		if err != nil {
			return err
		}
	}
	err = packet.Flush()
	if err != nil {
		return err
	}

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

type mraWriter struct {
	wr  io.Writer
	buf bytes.Buffer
}

func (p *mraWriter) WriteHeader(seq, msg, dlen uint32) error {
	return writeHeader(&p.buf, seq, msg, dlen)
}

func (p *mraWriter) WriteData(v ...interface{}) (err error) {
	for _, v := range v {
		err = packData(&p.buf, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *mraWriter) Flush() error {
	n, err := p.buf.WriteTo(p.wr)
	log.Printf("packet: send %d bytes\n", n)
	return err
}

func (p *mraWriter) Reset(w io.Writer) {
	p.wr = w
	p.buf.Reset()
}

var headerReserved = make([]byte, 16) // not used, must be filled with zeroes

func writeHeader(w io.Writer, seq, msg, dlen uint32) (err error) {
	err = binary.Write(w, binary.LittleEndian, mrim.CSMagic)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, mrim.ProtoVersion)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}

	var from, fromPort uint32 // not used, must be zero
	err = binary.Write(w, binary.LittleEndian, from)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, fromPort)
	if err != nil {
		return err
	}

	_, err = w.Write(headerReserved)
	return err
}

func readHeader(r io.Reader, seq, msg, dlen *uint32) (err error) {
	var magic, version uint32
	err = binary.Read(r, binary.LittleEndian, &magic)
	if err != nil {
		return err
	}
	if magic != mrim.CSMagic {
		return fmt.Errorf("wrong magic: %08x", magic)
	}
	err = binary.Read(r, binary.LittleEndian, &version)
	if err != nil {
		return err
	}
	//log.Printf("read head: version %d\n", version)
	err = binary.Read(r, binary.LittleEndian, seq)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, msg)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, dlen)
	if err != nil {
		return err
	}
	return nil
}

func readBody(conn io.ReadWriteCloser, dlen uint32) (err error) {
	// TODO(varankinv):
	buf := make([]byte, dlen+65000)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	log.Printf("read body: %d bytes, dlen %d, %q\n", n, dlen, buf[:n])
	return nil
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
