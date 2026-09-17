package transport

import "context"

type commands interface {
	isCommand()
}

type connectCommand struct {
	peerID int
	addr   string
}

func (connectCommand) isCommand() {
}

type Transport struct {
	commands chan commands
}

func NewTransport() Transport {
	return Transport{
		commands: make(chan commands, 100),
	}
}

func (t *Transport) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-t.commands:
			//Do something
			println(cmd)
		}
	}
}

func (t *Transport) Connect(peerID int, addr string) {
	t.commands <- connectCommand{
		peerID: peerID,
		addr:   addr,
	}
}
