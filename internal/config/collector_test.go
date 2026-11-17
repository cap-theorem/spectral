package config

import (
	"testing"

	"github.com/alecthomas/kong"
)

func TestCollectorCLIConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"defaults", nil, false},
		{"custom listen", []string{"--listen-addr", "127.0.0.1:9000"}, false},
		{"zero interval", []string{"--analyze-every", "0s"}, true},
		{"stale shorter than keepalive", []string{"--stale-after", "20s"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg CollectorCLIConfig
			parser, err := kong.New(&cfg)
			if err != nil {
				t.Fatal(err)
			}
			_, err = parser.Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}
