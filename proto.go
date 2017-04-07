package mrim

const (
	ProtoVersionMajor = 1
	ProtoVersionMinor = 8
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
	StatusOffline uint32 = iota
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
	MrimCSWPRequestParamUser uint = iota
	MrimCSWPRequestParamDomain
	MrimCSWPRequestParamNickname
	MrimCSWPRequestParamFirstname
	MrimCSWPRequestParamLastname
	MrimCSWPRequestParamSex
	MrimCSWPRequestParamBirthday
)
