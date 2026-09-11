package canvas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/webapp"
)

func archiveDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

var (
	ErrInstallURLInvalid = errors.New("invalid canvas install URL")
	ErrInstallURLBlocked = errors.New("canvas install URL destination is not public")
	ErrInstallDownload   = errors.New("canvas install download failed")
)

const (
	installDownloadTimeout = 2 * time.Minute
	installMaxRedirects    = 5
)

func newDistributionHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           publicDistributionDialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   2,
		DisableCompression:    false,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   installDownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= installMaxRedirects {
				return ErrInstallURLInvalid
			}
			if _, err := validateInstallURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

func (s *DistributionService) downloadInstallBundle(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := validateInstallURL(rawURL)
	if err != nil {
		return nil, err
	}
	s.installMu.Lock()
	client := s.httpClient
	s.installMu.Unlock()
	if client == nil {
		return nil, ErrInstallDownload
	}
	requestContext, cancel := context.WithTimeout(ctx, installDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: request could not be created", ErrInstallURLInvalid)
	}
	req.Header.Set("Accept", "application/gzip, application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstallDownload, safeDownloadError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: server returned %s", ErrInstallDownload, resp.Status)
	}
	if resp.ContentLength > webapp.MaxCompressedBytes {
		return nil, webapp.ErrCompressedTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, webapp.MaxCompressedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: response could not be read", ErrInstallDownload)
	}
	if int64(len(data)) > webapp.MaxCompressedBytes {
		return nil, webapp.ErrCompressedTooLarge
	}
	return data, nil
}

func validateInstallURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return nil, ErrInstallURLInvalid
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, ErrInstallURLInvalid
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicInstallIP(ip) {
		return nil, ErrInstallURLBlocked
	}
	return parsed, nil
}

func publicDistributionDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInstallURLBlocked
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicInstallIP(ip) {
			return nil, ErrInstallURLBlocked
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, ErrInstallURLBlocked
	}
	var lastErr error
	for _, ip := range ips {
		if !isPublicInstallIP(ip) {
			continue
		}
		connection, dialErr := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrInstallURLBlocked
}

func isPublicInstallIP(value net.IP) bool {
	parsed, err := netip.ParseAddr(value.String())
	if err != nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
		return false
	}
	if parsed.Is4() {
		value4 := parsed.As4()
		if value4[0] == 100 && value4[1] >= 64 && value4[1] <= 127 {
			return false
		}
	}
	return true
}

func safeDownloadError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New("remote download request failed")
}
