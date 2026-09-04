package overlay

type Hash []byte

type EpochRef struct {
	Sequence uint64
	Hash     Hash
}

type Epoch struct {
	Ref      EpochRef
	Prime    uint64
	PrevHash Hash
	Nodes    []Node   // Nodes in this epoch sequence
	Owners   []NodeId // NodeId of VVertex N is Owners[N]
}
