package wire

import (
	"net/netip"
	"uuid"
)

type NodeID = uuid.UUID

type Peer struct {
	ID      NodeID         `json:"id"`
	Address netip.AddrPort `json:"address"`
}
