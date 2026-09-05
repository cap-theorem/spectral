package identity

import "uuid"

type NodeId uuid.UUID

func NewNodeId() NodeId {
	return NodeId(uuid.NewV4())
}
