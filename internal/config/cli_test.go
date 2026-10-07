package config

import (
	"testing"

	"github.com/alecthomas/kong"
)

func TestDaemonCLIConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"defaults", []string{"--listen-addr", "127.0.0.1:7000"}, false},
		{"debug log level", []string{"--listen-addr", "127.0.0.1:7000", "--log-level", "debug"}, false},
		{"missing listen addr", []string{}, true},
		{"api port 0", []string{"--listen-addr", "127.0.0.1:7000", "--api-port", "0"}, true},
		{"bad bootstrap", []string{"--listen-addr", "127.0.0.1:7000", "--bootstrap", "nope"}, true},
		{"negative drain", []string{"--listen-addr", "127.0.0.1:7000", "--drain-timeout", "-1s"}, true},
		{"bad log level", []string{"--listen-addr", "127.0.0.1:7000", "--log-level", "nope"}, true},
		{"bad log format", []string{"--listen-addr", "127.0.0.1:7000", "--log-format", "xml"}, true},
		{"ipv6 listen addr", []string{"--listen-addr", "[fd7a:115c:a1e0::1]:7000"}, false},
		{"unspecified listen addr", []string{"--listen-addr", "0.0.0.0:7000"}, true},
		{"unspecified ipv6 listen addr", []string{"--listen-addr", "[::]:7000"}, true},
		{"listen port 0", []string{"--listen-addr", "127.0.0.1:0"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg DaemonCLIConfig
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
