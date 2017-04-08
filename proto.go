package mrim

const (
	protoVersionMajor = 1
	protoVersionMinor = 8
)

const (
	ProtoVersion uint32 = (protoVersionMajor << 16) | protoVersionMinor
	CSMagic      uint32 = 0xDEADBEEF
)

const (
	MsgCSHello                 = 0x1001
	MsgCSHelloAck              = 0x1002
	MsgCSLoginAck              = 0x1004
	MsgCSLoginRej              = 0x1005
	MsgCSPing                  = 0x1006
	MsgCSMessage              = 0x1008
	MsgCSMessageAck           = 0x1009
	MsgCSUserStatus           = 0x100F
	MsgCSMessageRecv          = 0x1011
	MsgCSMessageStatus        = 0x1012
	MsgCSLogout               = 0x1013
	MsgCSConnectionParams     = 0x1014
	MsgCSUserInfo              = 0x1015
	MsgCSAddContact           = 0x1019
	MsgCSAddContactAck        = 0x101A
	MsgCSModifyContact        = 0x101B
	MsgCSModifyContactAck     = 0x101C
	MrimCSOfflineMessageAck    = 0x101D
	MsgCSDeleteOfflineMessage = 0x101E
	MsgCSGetMpopSession       = 0x1024
	MsgCSMpopSession          = 0x1025
	MsgCSAnketaInfo           = 0x1028
	MsgCSWPRequest            = 0x1029
	MsgCSMailboxStatus        = 0x1033
	MsgCSContactList2          = 0x1037
	MsgCSLogin2                = 0x1038
)

const (
	StatusOffline        = 0x00000000
	StatusOnline         = 0x00000001
	StatusAway           = 0x00000002
	StatusUndeterminated = 0x00000003
	StatusFlagInvisible  = 0x80000000
)

const (
	MessageFlagOffline   = 0x00000001
	MessageFlagNorecv    = 0x00000004
	MessageFlagAuthorize = 0x00000008
)

const (
	mrimCSWPRequestParamUser      uint = iota
	mrimCSWPRequestParamDomain
	mrimCSWPRequestParamNickname
	mrimCSWPRequestParamFirstname
	mrimCSWPRequestParamLastname
	mrimCSWPRequestParamSex
	mrimCSWPRequestParamBirthday
)
