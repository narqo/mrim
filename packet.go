package mrim

import (
	"bytes"
	"encoding/binary"
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

type packetWriter struct {
	b bytes.Buffer
}

func (w *packetWriter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}

func (w *packetWriter) WriteData(v interface{}) (int, error) {
	return 0, writeData(&w.b, v)
}

func (w *packetWriter) Packet(msg uint32) (p Packet) {
	p.Data = w.b.Bytes()
	p.Header.Len = uint32(len(p.Data))
	p.Header.Msg = msg
	return
}

func writeData(w io.Writer, v interface{}) error {
	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case int:
		return binary.Write(w, binary.LittleEndian, uint32(v))
	case uint:
		return binary.Write(w, binary.LittleEndian, uint32(v))
	case uint32:
		return binary.Write(w, binary.LittleEndian, v)
	case string:
		err := binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(v))
		if err != nil {
			return err
		}
	case []byte:
		err := binary.Write(w, binary.LittleEndian, uint32(len(v)))
		if err != nil {
			return err
		}
		_, err = w.Write(v)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	return nil
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

func readPacketHeader(r io.Reader, p *Packet) (err error) {
	var magic, version uint32
	err = binary.Read(r, binary.LittleEndian, &magic)
	if err != nil {
		return err
	}
	if magic != CSMagic {
		return fmt.Errorf("wrong magic: %08x", magic)
	}
	err = binary.Read(r, binary.LittleEndian, &version)
	if err != nil {
		return err
	}

	err = binary.Read(r, binary.LittleEndian, &p.Seq)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, &p.Msg)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.LittleEndian, &p.Len)
	if err != nil {
		return err
	}
	return nil
}
