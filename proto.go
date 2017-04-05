package mrim

const (
	ProtoVersionMajor = 1
	ProtoVersionMinor = 22
)

const (
	ProtoVersion uint32 = (ProtoVersionMajor << 16) | ProtoVersionMinor
	ClientMagic  uint32 = 0xDEADBEEF
)

const (
	MrimCsHello          = 0x1001
	MrimCsHelloAck       = 0x1002
	MrimCsLoginAck       = 0x1004
	MrimCsLoginRej       = 0x1005
	MrimCsPing           = 0x1006
	MrimCsMessage        = 0x1008
	MrimCsMessageAck     = 0x1009
	MrimCsMessageRecv    = 0x1011
	MrimCsMessageStatus  = 0x1012
	MrimCsUserStatus     = 0x100F
	MrimCsLogout         = 0x1013
	MrimCsGetMpopSession = 0x1024
	MrimCsMpopSession    = 0x1025
)

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

func NewHeader(seq, msg, dlen uint32) *Header {
	return &Header{
		Magic:   ClientMagic,
		Proto:   ProtoVersion,
		Seq:     seq,
		Msg:     msg,
		DataLen: dlen,
	}
}
