package wire

import (
	"encoding/json"
	"errors"
)

var (
	ErrUnknownKind   = errors.New("unknown kind")
	ErrFrameTooLarge = errors.New("frame too large")
)

const (
	MaxFrame = 64 << 10 // 64 KiB max frame size
)

type Envelope struct {
	From Peer
	To   NodeID
	Msg  Message
}

type frame struct {
	Kind Kind            `json:"kind"`
	From Peer            `json:"from"`
	To   NodeID          `json:"to"`
	Body json.RawMessage `json:"body"`
}

func Encode(env Envelope) ([]byte, error) {
	body, err := json.Marshal(env.Msg)
	if err != nil {
		return nil, err
	}

	f := frame{
		Kind: env.Msg.Kind(),
		From: env.From,
		To:   env.To,
		Body: body,
	}

	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFrame {
		return nil, ErrFrameTooLarge
	}

	return b, nil
}

func Decode(b []byte) (Envelope, error) {
	if len(b) > MaxFrame {
		return Envelope{}, ErrFrameTooLarge
	}

	var f frame
	if err := json.Unmarshal(b, &f); err != nil {
		return Envelope{}, err
	}

	msg, err := decodeBody(f.Kind, f.Body)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{From: f.From, To: f.To, Msg: msg}, nil
}

func decodeBody(k Kind, body []byte) (Message, error) {
	switch k {
	case KindHeartbeat:
		var m Heartbeat
		err := json.Unmarshal(body, &m)
		return m, err
	case KindLink:
		var m Link
		err := json.Unmarshal(body, &m)
		return m, err
	case KindLinkAck:
		var m LinkAck
		err := json.Unmarshal(body, &m)
		return m, err
	case KindUnlink:
		var m Unlink
		err := json.Unmarshal(body, &m)
		return m, err
	case KindUpdate:
		var m Update
		err := json.Unmarshal(body, &m)
		return m, err
	case KindAEDigest:
		var m AEDigest
		err := json.Unmarshal(body, &m)
		return m, err
	case KindAEHeads:
		var m AEHeads
		err := json.Unmarshal(body, &m)
		return m, err
	case KindAEWant:
		var m AEWant
		err := json.Unmarshal(body, &m)
		return m, err
	case KindPing:
		var m Ping
		err := json.Unmarshal(body, &m)
		return m, err
	case KindPong:
		var m Pong
		err := json.Unmarshal(body, &m)
		return m, err
	case KindPingReq:
		var m PingReq
		err := json.Unmarshal(body, &m)
		return m, err
	case KindPingReqAck:
		var m PingReqAck
		err := json.Unmarshal(body, &m)
		return m, err
	default:
		return nil, ErrUnknownKind
	}
}
