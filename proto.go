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
	mrimCSHello                = 0x1001
	mrimCSHelloAck             = 0x1002
	mrimCSLoginAck             = 0x1004
	mrimCSLoginRej             = 0x1005
	mrimCSPing                 = 0x1006
	mrimCSMessage              = 0x1008
	mrimCSMessageAck           = 0x1009
	mrimCSUserStatus           = 0x100F
	mrimCSMessageRecv          = 0x1011
	mrimCSMessageStatus        = 0x1012
	mrimCSLogout               = 0x1013
	mrimCSConnectionParams     = 0x1014
	mrimCSUserInfo             = 0x1015
	mrimCSAddContact           = 0x1019
	mrimCSAddContactAck        = 0x101A
	mrimCSModifyContact        = 0x101B
	mrimCSModifyContactAck     = 0x101C
	mrimCSOfflineMessageAck    = 0x101D
	mrimCSDeleteOfflineMessage = 0x101E
	mrimCSGetMpopSession       = 0x1024
	mrimCSMpopSession          = 0x1025
	mrimCSAnketaInfo           = 0x1028
	mrimCSWPRequest            = 0x1029
	mrimCSMailboxStatus        = 0x1033
	mrimCSContactList2         = 0x1037
	mrimCSLogin2               = 0x1038
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
