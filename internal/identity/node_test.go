package identity_test

import (
	"testing"
	"uuid"

	"github.com/cap-theorem/spectral/internal/identity"
)

func TestNewIdentity(t *testing.T) {
	identity := identity.NewIdentity("127.0.0.1:3000")
	if identity.NodeId == uuid.Nil() {
		t.Error("Got a nil NodeId")
		return
	}
}
