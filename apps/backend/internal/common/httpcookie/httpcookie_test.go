package httpcookie

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(host, xfh string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = host
	if xfh != "" {
		r.Header.Set("X-Forwarded-Host", xfh)
	}
	return r
}

// tlsReq builds a request over TLS (direct), like req but with r.TLS set.
func tlsReq(host, xfh string) *http.Request {
	r := req(host, xfh)
	r.TLS = &tls.ConnectionState{}
	return r
}

// xfpReq builds a request marked https via X-Forwarded-Proto (proxied TLS).
func xfpReq(host, xfh string) *http.Request {
	r := req(host, xfh)
	r.Header.Set("X-Forwarded-Proto", "https")
	return r
}

func TestPortSuffix(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"ported host", req("127.0.0.1:8443", ""), "_8443"},
		{"no port", req("example.com", ""), ""},
		{"default port 80 http", req("example.com:80", ""), ""},
		{"default port 443 https", tlsReq("example.com:443", ""), ""},
		{"default port 443 https via xfp", xfpReq("example.com:443", ""), ""},
		{"non-default 443 on http", req("example.com:443", ""), "_443"},
		{"non-default 80 on https", tlsReq("example.com:80", ""), "_80"},
		{"XFH wins over host", req("internal:38429", "public.example:8443"), "_8443"},
		{"XFH wins on conflicting ports", req("public.example:8443", "public.example:9443"), "_9443"},
		{"XFH comma separated first", req("internal:38429", "public.example:8443, other:9443"), "_8443"},
		{"XFH whitespace padded", req("internal:38429", "  public.example:8443 , other:9443"), "_8443"},
		{"XFH whitespace only", req("internal:38429", "   "), ""},
		{"XFH no port", req("internal:38429", "public.example"), ""},
		{"XFH scheme-aware default", tlsReq("internal:38429", "public.example:443"), ""},
		{"IPv6 bracket", req("[::1]:8080", ""), "_8080"},
		{"IPv6 zone", req("[fe80::1%25eth0]:8080", ""), "_8080"},
		{"service name port", req("example.com:http", ""), ""},
		{"out of range port", req("example.com:99999", ""), ""},
		{"zero port", req("example.com:0", ""), ""},
		{"negative port", req("example.com:-1", ""), ""},
		{"malformed host", req("example.com:abc:def", ""), ""},
		{"unbracketed ipv6", req("::1", ""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PortSuffix(tt.req); got != tt.want {
				t.Fatalf("PortSuffix(%q) = %q, want %q", tt.req.Host, got, tt.want)
			}
		})
	}
}

func TestPortSuffixNilRequest(t *testing.T) {
	if got := PortSuffix(nil); got != "" {
		t.Fatalf("PortSuffix(nil) = %q, want empty", got)
	}
}

func TestScopedName(t *testing.T) {
	if got := ScopedName(req("127.0.0.1:8443", ""), "kandev_session"); got != "kandev_session_8443" {
		t.Fatalf("ScopedName ported = %q, want kandev_session_8443", got)
	}
	if got := ScopedName(req("example.com", ""), "kandev_session"); got != "kandev_session" {
		t.Fatalf("ScopedName no-port = %q, want kandev_session", got)
	}
	if got := ScopedName(req("127.0.0.1:8443", ""), ""); got != "" {
		t.Fatalf("ScopedName empty base = %q, want empty", got)
	}
}
