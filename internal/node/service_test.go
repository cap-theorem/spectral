package node

import (
	"errors"
	"testing"
	"time"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func settle(nodes ...*Node) {
	for range 5 {
		for _, n := range nodes {
			for drained := false; !drained; {
				select {
				case env := <-n.transport.Recv():
					n.handleEnvelope(env)
				default:
					drained = true
				}
			}
			n.flush()
		}
	}
}

func TestRegisterAdvertisesService(t *testing.T) {
	n := testNode()
	reg := testRegistration()
	inc := n.reg.records[n.self.ID].Inc

	lease, err := n.registerService(reg)
	if err != nil || lease.Token == "" {
		t.Fatalf("registerService() = %+v, %v", lease, err)
	}

	own := n.reg.records[n.self.ID]
	if own.Service.Name != reg.Service || own.Service.Endpoint != reg.Endpoint || own.Inc != inc+1 || own.Status != wire.StatusAlive {
		t.Fatalf("own record = %+v", own.Record)
	}
	if len(n.mem.outbox) == 0 {
		t.Fatal("registration not queued to flood")
	}

	got, err := n.lookupService(reg.Service)
	if err != nil || got.Endpoint != reg.Endpoint {
		t.Fatalf("lookupService() = %+v, %v", got, err)
	}
}

func TestLookupServiceNotFound(t *testing.T) {
	n := testNode()
	if _, err := n.lookupService("missing"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("lookupService() error = %v, want ErrServiceNotFound", err)
	}
}

func TestRenewService(t *testing.T) {
	tests := []struct {
		name    string
		token   func(lease Lease) string
		expires time.Duration
		wantErr error
	}{
		{"valid token", func(l Lease) string { return l.Token }, time.Second, nil},
		{"wrong token", func(l Lease) string { return "nope" }, time.Second, ErrLeaseNotFound},
		{"empty token", func(l Lease) string { return "" }, time.Second, ErrLeaseNotFound},
		{"expired lease", func(l Lease) string { return l.Token }, -time.Second, ErrLeaseNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode()
			lease, err := n.registerService(testRegistration())
			if err != nil {
				t.Fatal(err)
			}
			before := time.Now().Add(tt.expires)
			n.lease.ExpiresAt = before

			got, err := n.renewService(tt.token(lease))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("renewService() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !got.ExpiresAt.After(before) {
				t.Fatalf("expiry = %v, want after %v", got.ExpiresAt, before)
			}
			if tt.wantErr != nil && !n.lease.ExpiresAt.Equal(before) {
				t.Fatalf("failed renew moved expiry to %v", n.lease.ExpiresAt)
			}
		})
	}
}

func TestLeaseExpiryWithdrawsService(t *testing.T) {
	n := testNode()
	lease, err := n.registerService(testRegistration())
	if err != nil {
		t.Fatal(err)
	}
	inc := n.reg.records[n.self.ID].Inc

	n.expireLease(lease.ExpiresAt.Add(-time.Millisecond))
	if n.reg.records[n.self.ID].Service.Name == "" {
		t.Fatal("service withdrawn before expiry")
	}

	n.expireLease(lease.ExpiresAt)
	own := n.reg.records[n.self.ID]
	if own.Service.Name != "" || own.Status != wire.StatusAlive || own.Inc != inc+1 || n.lease != nil {
		t.Fatalf("after expiry: %+v, lease %v", own.Record, n.lease)
	}
	if _, err := n.lookupService(testRegistration().Service); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("lookupService() error = %v, want ErrServiceNotFound", err)
	}
	if _, err := n.renewService(lease.Token); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("renewService() error = %v, want ErrLeaseNotFound", err)
	}
}

func TestServiceLifecycleOverMem(t *testing.T) {
	network := transport.NewMemNetwork()
	a, b, c := memNode(network, 7601), memNode(network, 7602), memNode(network, 7603)
	now := time.Now()
	for _, n := range []*Node{a, b, c} {
		for _, m := range []*Node{a, b, c} {
			if n != m {
				linkTo(n, m.self, now)
			}
		}
	}

	reg := testRegistration()
	lease, err := a.registerService(reg)
	if err != nil {
		t.Fatal(err)
	}
	settle(a, b, c)

	for _, n := range []*Node{b, c} {
		got, err := n.lookupService(reg.Service)
		if err != nil || got.Endpoint != reg.Endpoint {
			t.Fatalf("lookup on %v = %+v, %v", n.self.Address, got, err)
		}
	}

	if _, err := a.renewService(lease.Token); err != nil {
		t.Fatalf("renewService() error = %v", err)
	}
	a.expireLease(time.Now().Add(time.Hour))
	settle(a, b, c)

	for _, n := range []*Node{b, c} {
		if _, err := n.lookupService(reg.Service); !errors.Is(err, ErrServiceNotFound) {
			t.Fatalf("lookup on %v after expiry: %v", n.self.Address, err)
		}
		if st := n.reg.records[a.self.ID].Status; st != wire.StatusAlive {
			t.Fatalf("expiry changed node status to %v", st)
		}
	}
}
