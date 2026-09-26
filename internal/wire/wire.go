package wire

import "uuid"

type NodeID uuid.UUID

type Peer struct {
	ID NodeID `json:"id"`

	// host:port format
	Address string `json:"address"`
}
