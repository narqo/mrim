package mrim

import (
	"encoding/binary"
	"fmt"
	"io"
)

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
