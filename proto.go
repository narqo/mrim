package mrim

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	ProtoVersionMajor = 1
	ProtoVersionMinor = 22
)

const (
	ProtoVersion uint32 = (ProtoVersionMajor << 16) | ProtoVersionMinor
	CSMagic      uint32 = 0xDEADBEEF
)

const (
	MrimCSHello          = 0x1001
	MrimCSHelloAck       = 0x1002
	MrimCSLoginAck       = 0x1004
	MrimCSLoginRej       = 0x1005
	MrimCSPing           = 0x1006
	MrimCSMessage        = 0x1008
	MrimCSMessageAck     = 0x1009
	MrimCSMessageRecv    = 0x1011
	MrimCSMessageStatus  = 0x1012
	MrimCSUserStatus     = 0x100F
	MrimCSLogout         = 0x1013
	MrimCSGetMpopSession = 0x1024
	MrimCSMpopSession    = 0x1025
	MrimCSLogin2         = 0x1038
)

const (
	StatusOffline        uint32 = iota
	StatusOnline
	StatusAway
	StatusUndeterminated
)

const StatusFlagInvisible = 0x80000000

const (
	MessageFlagOffline   = 0x00000001
	MessageFlagNorecv    = 0x00000004
	MessageFlagAuthorize = 0x00000008
)

const (
	MrimCSWPRequestParamUser      uint = iota
	MrimCSWPRequestParamDomain
	MrimCSWPRequestParamNickname
	MrimCSWPRequestParamFirstname
	MrimCSWPRequestParamLastname
	MrimCSWPRequestParamSex
	MrimCSWPRequestParamBirthday
)

type Header struct {
	// magic number
	Magic uint32
	// protocol version
	Proto uint32
	// sequence number
	Seq uint32
	// packet identifier
	Msg uint32
	// data length of the packet
	DataLen uint32
	// sender address (not used)
	From uint32
	// sender port (not used)
	FromPort uint32
	// must be filled with zeros
	Received [16]byte
}

func Pack(buf *bytes.Buffer, seq, msg, dlen uint32, val ...interface{}) error {
	packet := make([]byte, 44)

	b := packet
	binary.LittleEndian.PutUint32(b, CSMagic)
	b = b[4:]
	binary.LittleEndian.PutUint32(b, ProtoVersion)
	b = b[4:]
	binary.LittleEndian.PutUint32(b, seq)
	b = b[4:]
	binary.LittleEndian.PutUint32(b, msg)
	b = b[4:]
	binary.LittleEndian.PutUint32(b, dlen)

	_, err := buf.Write(packet)
	if err != nil {
		return err
	}

	for _, v := range val {
		packet = packet[:]
		if err := packOne(buf, packet, v); err != nil {
			return err
		}
	}
	return nil
}

func packOne(buf *bytes.Buffer, b []byte, v interface{}) error {
	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case uint32:
		binary.LittleEndian.PutUint32(b, v)
		if _, err := buf.Write(b[:4]); err != nil {
			return err
		}
	case int:
		binary.LittleEndian.PutUint32(b, uint32(v))
		if _, err := buf.Write(b[:4]); err != nil {
			return err
		}
	case string:
		binary.LittleEndian.PutUint32(b, uint32(len(v)))
		if _, err := buf.Write(b[:4]); err != nil {
			return err
		}
		if _, err := buf.WriteString(v); err != nil {
			return err
		}
	}
	return nil
}

func Unpack(packet []byte) (seq, msg uint32, buf []byte, err error) {
	if len(packet) < 44 {
		return 0, 0, nil, fmt.Errorf("packet too small: %d", len(packet))
	}
	magic, packet := binary.LittleEndian.Uint32(packet), packet[4:]
	if magic != CSMagic {
		return 0, 0, nil, fmt.Errorf("wrong magic: %08x", magic)
	}
	// skip proto version check
	_, packet = binary.LittleEndian.Uint32(packet), packet[4:]
	seq, packet = binary.LittleEndian.Uint32(packet), packet[4:]
	msg, packet = binary.LittleEndian.Uint32(packet), packet[4:]

	buf = packet

	return
}
