package wire

type Kind uint8

const (
	KindHeartbeat Kind = iota + 1
	KindLink
	KindLinkAck
	KindUnlink

	KindUpdate
	KindAEDigest
	KindAEHeads
	KindAEWant

	KindPing
	KindPong
	KindPingReq
	KindPingReqAck
)

type Message interface {
	Kind() Kind
}

type Status uint8

const (
	StatusAlive Status = iota
	StatusSuspect
	StatusDead
	StatusLeft
)

type Service struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type Record struct {
	Peer    Peer    `json:"peer"`
	Cap     int     `json:"cap"`
	Inc     uint64  `json:"inc"`
	Status  Status  `json:"status"`
	Service Service `json:"service"`
}

type Head struct {
	ID     NodeID `json:"id"`
	Inc    uint64 `json:"inc"`
	Status Status `json:"status"`
}

type Heartbeat struct {
	Want bool `json:"want"`
}

func (Heartbeat) Kind() Kind { return KindHeartbeat }

type Link struct{}

func (Link) Kind() Kind { return KindLink }

type LinkAck struct{}

func (LinkAck) Kind() Kind { return KindLinkAck }

type Unlink struct{}

func (Unlink) Kind() Kind { return KindUnlink }

type Update struct {
	Records []Record `json:"records"`
}

func (Update) Kind() Kind { return KindUpdate }

type AEDigest struct {
	Digest []uint64 `json:"digest"`
}

func (AEDigest) Kind() Kind { return KindAEDigest }

type AEHeads struct {
	Buckets []int  `json:"buckets"`
	Heads   []Head `json:"heads"`
}

func (AEHeads) Kind() Kind { return KindAEHeads }

type AEWant struct {
	IDs     []NodeID `json:"ids,omitempty"`
	Records []Record `json:"records,omitempty"`
}

func (AEWant) Kind() Kind { return KindAEWant }

type Ping struct {
	Nonce uint64 `json:"nonce"`
}

func (Ping) Kind() Kind { return KindPing }

type Pong struct {
	Nonce uint64 `json:"nonce"`
}

func (Pong) Kind() Kind { return KindPong }

type PingReq struct {
	Nonce  uint64 `json:"nonce"`
	Target Peer   `json:"target"`
}

func (PingReq) Kind() Kind { return KindPingReq }

type PingReqAck struct {
	Nonce  uint64 `json:"nonce"`
	Target NodeID `json:"target"`
}

func (PingReqAck) Kind() Kind { return KindPingReqAck }
