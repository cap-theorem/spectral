package overlay_test

import (
	"testing"
	"uuid"

	"github.com/cap-theorem/spectral/internal/overlay"
)

func TestNewMember(t *testing.T) {
	node := overlay.NewNode("127.0.0.1:3000")
	if node.ID == overlay.NodeId(uuid.Nil()) {
		t.Error("Got a nil NodeId")
		return
	}

	if node.Addr != "127.0.0.1:3000" {
		t.Errorf("Got %s instead of 127.0.0.1:3000", node.Addr)
		return
	}
}
