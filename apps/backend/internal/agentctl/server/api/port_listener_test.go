package api

import (
	"testing"
)

// TestParseProcNetLine covers the /proc/net/tcp row parser, which is the
// fallback path used inside minimal containers that ship no `ss`.
func TestParseProcNetLine(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantPort int
		wantAddr string
		wantOK   bool
	}{
		{
			name:     "listening ipv4 socket",
			line:     "  0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000 100 0 0 10 0",
			wantPort: 8080,
			wantAddr: "127.0.0.1",
			wantOK:   true,
		},
		{
			name:     "all interfaces",
			line:     "  1: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12346 1 0000 100 0 0 10 0",
			wantPort: 3000,
			wantAddr: "0.0.0.0",
			wantOK:   true,
		},
		{
			name:     "established socket is not a listener",
			line:     "  2: 0100007F:1F90 0100007F:9C40 01 00000000:00000000 00:00000000 00000000  1000        0 12347 1 0000 100 0 0 10 0",
			wantPort: 0,
			wantOK:   false,
		},
		{
			name:   "truncated row",
			line:   "  3: 0100007F:1F90 00000000:0000",
			wantOK: false,
		},
		{
			name:   "local address without a port",
			line:   "  4: 0100007F 00000000:0000 0A 00000000:00000000 00:00000000 00000000",
			wantOK: false,
		},
		{
			name:   "non-hex port",
			line:   "  5: 0100007F:ZZZZ 00000000:0000 0A 00000000:00000000 00:00000000 00000000",
			wantOK: false,
		},
		{
			name:   "blank line",
			line:   "",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port, addr, ok := parseProcNetLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (port %d, addr %q)", ok, tc.wantOK, port, addr)
			}
			if !tc.wantOK {
				return
			}
			if port != tc.wantPort {
				t.Errorf("port = %d, want %d", port, tc.wantPort)
			}
			if addr != tc.wantAddr {
				t.Errorf("address = %q, want %q", addr, tc.wantAddr)
			}
		})
	}
}

// TestParseHexIP pins the little-endian byte order /proc/net/tcp uses for IPv4.
// Reading it big-endian yields a plausible-looking but wrong address (1.0.0.127
// instead of 127.0.0.1), which no smoke test would catch.
func TestParseHexIP(t *testing.T) {
	cases := map[string]string{
		"0100007F":                         "127.0.0.1",
		"00000000":                         "0.0.0.0",
		"0201A8C0":                         "192.168.1.2",
		"00000000000000000000000000000000": defaultListenAddr, // IPv6 collapses to all-interfaces
		"0000000000000000FFFF00000100007F": defaultListenAddr, // IPv4-mapped IPv6
		"ZZZZ":                             defaultListenAddr, // undecodable
		"0100":                             defaultListenAddr, // too short for an address
		"":                                 defaultListenAddr,
	}
	for input, want := range cases {
		if got := parseHexIP(input); got != want {
			t.Errorf("parseHexIP(%q) = %q, want %q", input, got, want)
		}
	}
}
