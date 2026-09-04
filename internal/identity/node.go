package identity

import "uuid"

type Identity struct {
	NodeId  uuid.UUID
	Address string // The QUIC Address
}

func NewIdentity(quicAddr string) Identity {
	return Identity{
		NodeId:  uuid.NewV4(),
		Address: quicAddr,
	}
}
