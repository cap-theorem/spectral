package wire

type Kind uint8

const (
	KindHeartbeat Kind = iota + 1
	KindLink
	KindLinkAck
	KindLinkNack
	KindUnlink

	KindWalk
	KindWalkFail

	KindSplit
	KindSplitNack

	KindLeave

	KindRewire
	KindRewireNack
	KindSwitchAbort

	KindPublish
	KindQuery
	KindQueryHit
	KindQueryMiss
	KindPing
	KindPong
)

type Message interface {
	Kind() Kind
}

type Ping struct {
	Nonce uint64 `json:"nonce"`
}

func (Ping) Kind() Kind { return KindPing }

type Pong struct {
	Nonce uint64 `json:"nonce"`
}

func (Pong) Kind() Kind { return KindPong }
