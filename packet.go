package mrim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type PacketError struct {
	p   Packet
	err error
}

func (e PacketError) Error() string {
	return fmt.Sprintf("packet error: %v: %04x", e.err, e.p.Header.Msg)
}

type Header struct {
	// sequence of the packet used to wait for acknowledgement
	Seq uint32
	// identifier of the packet
	Msg uint32
	// data length
	Len uint32
}

type Packet struct {
	Header
	Data []byte
}

var headerReserved [16]byte // not used, must be filled with zeroes

func writePacket(w io.Writer, p Packet) (err error) {
	// write header
	err = binary.Write(w, binary.LittleEndian, CSMagic)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, ProtoVersion)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, p.Seq)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, p.Msg)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, p.Len)
	if err != nil {
		return err
	}

	var from, fromPort uint32 // not used, must be zeros
	err = binary.Write(w, binary.LittleEndian, from)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.LittleEndian, fromPort)
	if err != nil {
		return err
	}

	_, err = w.Write(headerReserved[:])
	if err != nil {
		return err
	}

	_, err = w.Write(p.Data)
	if err != nil {
		return err
	}
	return
}

const headerSize = 44

func readPacketHeader(buf []byte, p *Packet) (err error) {
	if len(buf) < headerSize {
		return fmt.Errorf("buffer too small: %d", len(buf))
	}

	magic := binary.LittleEndian.Uint32(buf[0:])
	if magic != CSMagic {
		return fmt.Errorf("wrong magic: %08x", magic)
	}

	version := binary.LittleEndian.Uint32(buf[4:])
	_ = version

	p.Seq = binary.LittleEndian.Uint32(buf[8:])
	p.Msg = binary.LittleEndian.Uint32(buf[12:])
	p.Len = binary.LittleEndian.Uint32(buf[16:])

	// the rest bytes are for from, fromPort, and headerReserved fields, which are not used

	return nil
}

type PacketWriter struct {
	b bytes.Buffer
}

func (w *PacketWriter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}

// maybe https://github.com/go-redis/redis/blob/master/internal/proto/scan.go
func (w *PacketWriter) WriteData(v interface{}) (n int, err error) {
	if v == nil {
		return
	}

	switch v := v.(type) {
	case int:
		err = binary.Write(w, binary.LittleEndian, uint32(v))
	case uint:
		err = binary.Write(w, binary.LittleEndian, uint32(v))
	case uint32:
		err = binary.Write(w, binary.LittleEndian, v)
	case []byte:
		err = binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return
		}
		n, err = w.Write(v)
		n += 4
	case string:
		err = binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return
		}
		n, err = w.Write([]byte(v))
		n += 4
	default:
		err = fmt.Errorf("unsupported type %T", v)
	}
	return
}

func (w *PacketWriter) Packet(msg uint32) (p Packet) {
	p.Data = w.b.Bytes()
	p.Header.Len = uint32(len(p.Data))
	p.Header.Msg = msg
	return
}

// unpackLPS unpacks LSP (long pascal string, size uint32 + str string) from v.
func unpackLPS(v []byte) (string, error) {
	if v == nil {
		return "", nil
	}
	l := binary.LittleEndian.Uint32(v)
	v = v[4:]
	if int(l) > len(v) {
		return "", errors.New("out of bound")
	}
	return string(v[:l]), nil
}
