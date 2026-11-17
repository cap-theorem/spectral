package collector

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/cap-theorem/spectral/internal/wire"
)

const ReportPath = "/v1/reports"

const maxLinks = 4096

var ErrInvalidReport = errors.New("invalid report")

// Report holds all of a node's current links and replaces its previous report.
type Report struct {
	ID      wire.NodeID    `json:"id"`
	Address netip.AddrPort `json:"address"`
	Seq     uint64         `json:"seq"` // reports with Seq <= the last applied one are ignored
	Left    bool           `json:"left,omitempty"`
	Links   []ReportLink   `json:"links"`
}

type ReportLink struct {
	Peer wire.NodeID `json:"peer"`
	Out  bool        `json:"out"` // the reporter wants this link, not only the peer
}

func (r Report) Validate() error {
	if r.ID == (wire.NodeID{}) {
		return fmt.Errorf("%w: id is required", ErrInvalidReport)
	}
	if len(r.Links) > maxLinks {
		return fmt.Errorf("%w: %d links exceeds limit %d", ErrInvalidReport, len(r.Links), maxLinks)
	}
	for _, l := range r.Links {
		if l.Peer == (wire.NodeID{}) || l.Peer == r.ID {
			return fmt.Errorf("%w: link peer must be another node's id", ErrInvalidReport)
		}
	}
	return nil
}
