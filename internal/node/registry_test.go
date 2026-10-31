package node

import (
	"net/netip"
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/wire"
)

func TestRegistryMerge(t *testing.T) {
	peer := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("10.0.0.1:7000")}
	rec := func(inc uint64, st wire.Status) *wire.Record {
		return &wire.Record{Peer: peer, Cap: 8, Inc: inc, Status: st}
	}

	tests := []struct {
		name string
		have *wire.Record
		in   *wire.Record
		want bool
	}{
		{"unknown id", nil, rec(0, wire.StatusAlive), true},
		{"higher inc", rec(1, wire.StatusDead), rec(2, wire.StatusAlive), true},
		{"worse status same inc", rec(1, wire.StatusAlive), rec(1, wire.StatusSuspect), true},
		{"same record", rec(1, wire.StatusSuspect), rec(1, wire.StatusSuspect), false},
		{"better status same inc", rec(1, wire.StatusSuspect), rec(1, wire.StatusAlive), false},
		{"lower inc", rec(2, wire.StatusAlive), rec(1, wire.StatusDead), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry()
			if tt.have != nil {
				r.merge(*tt.have, time.Unix(1, 0))
			}

			if got := r.merge(*tt.in, time.Unix(2, 0)); got != tt.want {
				t.Fatalf("merge() = %v, want %v", got, tt.want)
			}

			want := tt.in
			if !tt.want {
				want = tt.have
			}
			if got := r.records[peer.ID].Record; !reflect.DeepEqual(got, *want) {
				t.Fatalf("got %+v, want %+v", got, *want)
			}
		})
	}
}

func TestRegistryLookup(t *testing.T) {
	r := newRegistry()
	add := func(name string, st wire.Status) wire.Service {
		svc := wire.Service{Name: name, Endpoint: "http://" + uuid.NewV4().String() + ":8080"}
		r.merge(wire.Record{Peer: wire.Peer{ID: uuid.NewV4()}, Status: st, Service: svc}, time.Now())
		return svc
	}

	alive := add("payments", wire.StatusAlive)
	add("payments", wire.StatusSuspect)
	add("payments", wire.StatusDead)
	add("orders", wire.StatusAlive)

	got := r.lookup("payments")
	if len(got) != 1 || got[0] != alive {
		t.Fatalf("lookup(payments) = %v, want [%v]", got, alive)
	}
	if got := r.lookup("missing"); len(got) != 0 {
		t.Fatalf("lookup(missing) = %v, want none", got)
	}
}
