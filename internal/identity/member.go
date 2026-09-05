package identity

type Member struct {
	NodeId NodeId
	Addr   string
}

type MemberId struct {
	NodeId NodeId
}

func (m *Member) Id() NodeId {
	return m.NodeId
}

func (m *MemberId) Id() NodeId {
	return m.NodeId
}
