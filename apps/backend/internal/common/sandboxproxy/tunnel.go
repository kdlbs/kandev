package sandboxproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	target, ok := p.destination(r)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	defer cancel()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = upstream.Close() }()
	client, buffer, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	if !p.retain(client, upstream) {
		_ = client.Close()
		return
	}
	defer p.release(client, upstream)
	if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err = buffer.Flush(); err != nil {
		return
	}
	if err = client.SetDeadline(time.Time{}); err != nil {
		return
	}
	copied := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, buffer); closeWrite(upstream); close(copied) }()
	_, _ = io.Copy(client, upstream)
	closeWrite(client)
	_ = upstream.Close()
	_ = client.Close()
	<-copied
}

func closeWrite(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}

func (p *Proxy) destination(r *http.Request) (string, bool) {
	if p.ctx.Err() != nil || r.ContentLength > 0 || len(r.TransferEncoding) > 0 || r.URL.User != nil || r.URL.RawQuery != "" || r.URL.Path != "" || r.Host != r.RequestURI {
		return "", false
	}
	host, portText, err := net.SplitHostPort(r.RequestURI)
	if err != nil {
		return "", false
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 || strconv.FormatUint(port, 10) != portText {
		return "", false
	}
	target := net.JoinHostPort(strings.ToLower(host), portText)
	return target, p.allowed[target]
}
