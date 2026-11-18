package node

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"net/netip"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
	"go.opentelemetry.io/otel/metric"
)

const (
	tickInterval = 20 * time.Millisecond
	pingTimeout  = 2 * time.Second
)

type Node struct {
	self  wire.Peer
	state State

	lease *Lease

	reg registry
	ov  overlay
	mem membership

	inbox       chan request
	transport   transport.Transport
	logger      *slog.Logger
	metrics     NodeMetrics
	pendingPing map[uint64]pendingPing

	telemetry  chan collector.Report
	lastReport []collector.ReportLink
	nextReport time.Time
	keepalive  time.Time
}

type NodeMetrics struct {
	PongsReceived metric.Int64Counter
}

type pendingPing struct {
	peerID   wire.NodeID
	started  time.Time
	deadline time.Time
	reply    chan result[time.Duration]
}

type NodeConfig struct {
	ListenAddr netip.AddrPort
	Seeds      []string
	Overlay    OverlayKind
	Metrics    NodeMetrics
	Telemetry  chan collector.Report
}

func NewNode(cfg NodeConfig, tr transport.Transport, logger *slog.Logger) *Node {
	self := wire.Peer{
		ID:      uuid.NewV4(),
		Address: cfg.ListenAddr,
	}

	n := &Node{
		self:        self,
		state:       StateActive,
		lease:       nil,
		reg:         newRegistry(),
		ov:          newOverlay(self.ID, cfg.Overlay),
		mem:         newMembership(cfg.Seeds),
		inbox:       make(chan request, 100),
		transport:   tr,
		logger:      logger,
		metrics:     cfg.Metrics,
		pendingPing: make(map[uint64]pendingPing),
		telemetry:   cfg.Telemetry,
	}
	n.reg.merge(wire.Record{Peer: self, Status: wire.StatusAlive}, time.Now())

	return n
}

func (n *Node) Register(ctx context.Context, reg Registration) (Lease, error) {
	req := registerRequest{Registration: reg, Reply: make(chan result[Lease], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}
}

func (n *Node) Renew(ctx context.Context, token string) (Lease, error) {
	req := renewRequest{Token: token, Reply: make(chan result[Lease], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}
}

func (n *Node) Lookup(ctx context.Context, service string) (Provider, error) {
	req := lookupRequest{Service: service, Reply: make(chan result[Provider], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Provider{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Provider{}, ctx.Err()
	}
}

func (n *Node) Status(ctx context.Context) (Status, error) {
	req := statusRequest{Reply: make(chan result[Status], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
}

func (n *Node) Leave(ctx context.Context) error {
	req := leaveRequest{Reply: make(chan struct{}, 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-req.Reply:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Node) Run(ctx context.Context) {
	n.logger.Info("Node started", "id", n.self.ID, "address", n.self.Address)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case req := <-n.inbox:
			n.handleRequest(req)
		case env := <-n.transport.Recv():
			n.handleEnvelope(env)
		case now := <-ticker.C:
			n.tick(now)
		}
		n.flush()
	}
}

func (n *Node) tick(now time.Time) {
	if n.state == StateLeft {
		return
	}

	n.overlayTick(now)
	n.memberTick(now)
	n.expireLease(now)
	n.expirePings(now)
	n.reportTick(now)
}

func (n *Node) handleRequest(req request) {
	switch req := req.(type) {
	case registerRequest:
		n.logger.Info("registration received", "endpoint", req.Registration.Endpoint)
		lease, err := n.registerService(req.Registration)
		req.Reply <- result[Lease]{Value: lease, Err: err}
	case renewRequest:
		n.logger.Info("renew request received")
		lease, err := n.renewLease(req.Token)
		req.Reply <- result[Lease]{Value: lease, Err: err}
	case lookupRequest:
		n.logger.Info("lookup request received", "service", req.Service)
		req.Reply <- result[Provider]{Value: Provider{Service: req.Service}}
	case statusRequest:
		n.logger.Info("status request received")
		req.Reply <- result[Status]{Value: Status{Ready: n.state == StateActive, State: n.state}}
	case leaveRequest:
		n.logger.Info("leave request received")
		n.leave(time.Now())
		req.Reply <- struct{}{}
	case pingRequest:
		n.logger.Info("ping request received", "peer", req.ID)
		n.startPing(req)
	}
}

func (n *Node) handleEnvelope(env wire.Envelope) {
	if n.state == StateLeft {
		return
	}

	now := time.Now()
	n.ov.heard(env.From.ID, now)
	n.clearProbes(env.From.ID)

	switch msg := env.Msg.(type) {
	case wire.Ping:
		n.logger.Debug("ping received", "from", env.From.ID)
		n.send(env.From, wire.Pong{Nonce: msg.Nonce})
	case wire.Pong:
		if n.metrics.PongsReceived != nil {
			n.metrics.PongsReceived.Add(context.Background(), 1)
		}
		n.logger.Info("pong received", "from", env.From.ID, "address", env.From.Address)
		pending, ok := n.pendingPing[msg.Nonce]
		if ok && pending.peerID == env.From.ID && env.To == n.self.ID {
			delete(n.pendingPing, msg.Nonce)
			pending.reply <- result[time.Duration]{Value: time.Since(pending.started)}
		}
		if env.To == n.self.ID {
			n.handleRelayPong(env.From, msg)
		}
	case wire.Link:
		n.ov.addLink(env.From, true, now)
		n.send(env.From, wire.LinkAck{})
	case wire.LinkAck:
		n.ov.addLink(env.From, false, now)
	case wire.Unlink:
		delete(n.ov.links, env.From.ID)
	case wire.Heartbeat:
		l, ok := n.ov.links[env.From.ID]
		if !ok {
			if msg.Want {
				n.ov.addLink(env.From, true, now)
			}
			return
		}
		l.theirs = msg.Want
		n.ov.links[env.From.ID] = l
	case wire.Update:
		n.handleUpdate(env.From, msg, now)
	case wire.AEDigest:
		n.handleAEDigest(env.From, msg)
	case wire.AEHeads:
		n.handleAEHeads(env.From, msg, now)
	case wire.AEWant:
		n.handleAEWant(env.From, msg, now)
	case wire.PingReq:
		n.handlePingReq(env.From, msg, now)
	case wire.PingReqAck:
		n.handlePingReqAck(env.From, msg)
	}
}

func (n *Node) send(to wire.Peer, msg wire.Message) {
	n.transport.Send(to.Address, wire.Envelope{
		From: n.self,
		To:   to.ID,
		Msg:  msg,
	})
}

func (n *Node) registerService(reg Registration) (Lease, error) {
	if n.state == StateLeft {
		return Lease{}, ErrNodeLeft
	}
	if err := reg.Validate(); err != nil {
		return Lease{}, err
	}

	now := time.Now()

	token, err := generateHexToken(32)
	if err != nil {
		return Lease{}, ErrTokenGenerationFail
	}

	own := n.reg.records[n.self.ID].Record

	own.Service.Name = reg.Service
	own.Service.Endpoint = reg.Endpoint
	own.Inc++
	own.Status = wire.StatusAlive

	n.apply(own, n.self.ID, now)

	n.lease = &Lease{
		Token:     token,
		ExpiresAt: now.Add(reg.TTL),
		TTL:       reg.TTL,
	}

	return *n.lease, nil
}

func (n *Node) renewLease(token string) (Lease, error) {
	if n.state == StateLeft {
		return Lease{}, ErrNodeLeft
	}

	now := time.Now()
	if n.lease == nil || subtle.ConstantTimeCompare([]byte(token), []byte(n.lease.Token)) != 1 || !now.Before(n.lease.ExpiresAt) {
		return Lease{}, ErrLeaseNotFound
	}

	n.lease.ExpiresAt = now.Add(n.lease.TTL)
	return *n.lease, nil
}

func generateHexToken(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (n *Node) Ping(ctx context.Context, id wire.NodeID) (time.Duration, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	nonce := binary.LittleEndian.Uint64(b[:])

	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	req := pingRequest{
		ID:       id,
		Nonce:    nonce,
		Deadline: deadline,
		Reply:    make(chan result[time.Duration], 1),
	}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return 0, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (n *Node) startPing(req pingRequest) {
	e, ok := n.reg.records[req.ID]
	if !ok {
		req.Reply <- result[time.Duration]{Err: ErrUnknownPeer}
		return
	}

	n.pendingPing[req.Nonce] = pendingPing{
		peerID:   req.ID,
		started:  time.Now(),
		deadline: req.Deadline,
		reply:    req.Reply,
	}
	n.send(e.Peer, wire.Ping{Nonce: req.Nonce})
}

func (n *Node) expirePings(now time.Time) {
	for nonce, pending := range n.pendingPing {
		if now.After(pending.deadline) {
			delete(n.pendingPing, nonce)
		}
	}
}

func (n *Node) expireLease(now time.Time) {
	if n.lease == nil || now.Before(n.lease.ExpiresAt) {
		return
	}

	own := n.reg.records[n.self.ID].Record
	own.Service = wire.Service{}
	own.Inc++
	n.apply(own, n.self.ID, now)
	n.lease = nil
}
