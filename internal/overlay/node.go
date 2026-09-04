package overlay

import "uuid"

type NodeId uuid.UUID

type Node struct {
	ID   NodeId
	Addr string
}

func NewNode(addr string) Node {
	return Node{
		ID:   NodeId(uuid.NewV4()),
		Addr: addr,
	}
}
