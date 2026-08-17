package backendapp

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// trustedProxiesEnv lists the reverse proxies whose X-Forwarded-For header
// the backend trusts. Comma-separated IPs and CIDRs.
const trustedProxiesEnv = "KANDEV_TRUSTED_PROXIES"

// emptyTrustedProxyEntry is the placeholder name used when a blank component
// appears inside a nonblank KANDEV_TRUSTED_PROXIES value (e.g. a trailing or
// doubled comma). Blank is not a valid IP or CIDR, so it fails the whole list
// closed like any other unparsable entry.
const emptyTrustedProxyEntry = "<empty>"

const trustedProxiesWarningMsg = "KANDEV_TRUSTED_PROXIES contains invalid entries; ignoring X-Forwarded-For entirely"

// resolveTrustedProxies parses the comma-separated KANDEV_TRUSTED_PROXIES
// value into gin-compatible proxy origins (bare IPs or CIDRs). An entirely
// blank value means "no trusted proxies" (trusted nil, no invalid entries).
// Any unparsable entry, including a blank component inside a nonblank value,
// makes the whole list untrustworthy: trusted is nil then (fail closed) and
// invalid names the bad values for the startup warning. The returned strings
// are the original entries, already validated, so SetTrustedProxies cannot
// reject them.
func resolveTrustedProxies(raw string) (trusted []string, invalid []string) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	for _, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			invalid = append(invalid, emptyTrustedProxyEntry)
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				invalid = append(invalid, entry)
				continue
			}
		} else if net.ParseIP(entry) == nil {
			invalid = append(invalid, entry)
			continue
		}
		trusted = append(trusted, entry)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return trusted, nil
}

// configureTrustedProxies reads KANDEV_TRUSTED_PROXIES and applies it to the
// router. Unset, empty, or fully invalid lists disable forwarded-header trust
// (SetTrustedProxies(nil)); an invalid entry logs a warning naming the bad
// value(s). gin trusts all proxies out of the box, so without this call a
// directly reachable backend would accept a spoofed X-Forwarded-For and
// defeat the ClientIP-keyed login rate limiter.
func configureTrustedProxies(router *gin.Engine, log *logger.Logger) {
	trusted, invalid := resolveTrustedProxies(os.Getenv(trustedProxiesEnv))
	if len(invalid) > 0 {
		log.Warn(trustedProxiesWarningMsg, zap.Strings("invalid_entries", invalid))
		trusted = nil
	}
	if err := router.SetTrustedProxies(trusted); err != nil {
		log.Warn("failed to configure trusted proxies", zap.Error(err))
	}
}
