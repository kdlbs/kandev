package lifecycle

import (
	"path"
	"strings"
)

const (
	sshRemoteWindowsAgentctlPath = "~/.kandev/bin/agentctl.exe"
	sshRemoteWindowsAgentctlSha  = "~/.kandev/bin/agentctl.exe.sha256"
	MetadataKeySSHRemotePlatform = "ssh_remote_platform"
	defaultWindowsLoginShell     = "powershell"
	windowsPowerShellCoreBinary  = "pwsh"
)

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isWindowsRemotePlatform(platform SSHRemotePlatform) bool {
	return platform.GOOS == sshRemoteGOOSWindows
}

func (p SSHRemotePlatform) isKnown() bool {
	return p.GOOS != "" && p.GOARCH != ""
}

// isWindowsRemotePath reports whether p is a Windows drive-letter path
// (including SFTP-prefixed /C:/...) or UNC path (\\server\share\...).
func isWindowsRemotePath(p string) bool {
	p = strings.TrimSpace(p)
	return isWindowsDrivePath(p) || isSFTPWindowsDrivePath(p) || isWindowsUNCPath(p)
}

func isWindowsDrivePath(p string) bool {
	if len(p) < 2 || !isASCIIAlpha(p[0]) || p[1] != ':' {
		return false
	}
	return len(p) == 2 || p[2] == '\\' || p[2] == '/'
}

func isSFTPWindowsDrivePath(p string) bool {
	if len(p) < 3 || p[0] != '/' || !isASCIIAlpha(p[1]) || p[2] != ':' {
		return false
	}
	return len(p) == 3 || p[3] == '\\' || p[3] == '/'
}

func isWindowsUNCPath(p string) bool {
	if !strings.HasPrefix(p, `\\`) || len(p) <= 2 || p[2] == '\\' {
		return false
	}
	rest := p[2:]
	sep := strings.IndexAny(rest, `\/`)
	if sep <= 0 || sep+1 >= len(rest) {
		return false
	}
	next := rest[sep+1]
	return next != '\\' && next != '/'
}

// cleanWindowsRemotePath normalizes a Windows drive or UNC path independently
// of the control-plane OS. Drive letters are uppercased, separators become `\`,
// and `.`/`..` segments are resolved without crossing the volume root.
func cleanWindowsRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if len(p) >= 3 && p[0] == '/' && isASCIIAlpha(p[1]) && p[2] == ':' {
		p = p[1:]
	}
	replaced := strings.ReplaceAll(p, "/", `\`)
	prefix, rest := splitWindowsVolumePrefix(replaced)
	if prefix == "" {
		return cleanWindowsRelativeSegments(replaced)
	}
	segments := resolveWindowsSegments(rest, true)
	if len(segments) == 0 {
		if strings.HasPrefix(prefix, `\\`) {
			return prefix
		}
		return prefix + `\`
	}
	return prefix + `\` + strings.Join(segments, `\`)
}

func splitWindowsVolumePrefix(p string) (string, string) {
	if len(p) >= 2 && isASCIIAlpha(p[0]) && p[1] == ':' {
		drive := strings.ToUpper(p[:1]) + ":"
		return drive, strings.TrimLeft(p[2:], `\`)
	}
	if strings.HasPrefix(p, `\\`) && len(p) > 2 && p[2] != '\\' {
		trimmed := p[2:]
		firstSep := strings.IndexByte(trimmed, '\\')
		if firstSep <= 0 {
			return "", p
		}
		afterServer := trimmed[firstSep+1:]
		if afterServer == "" || afterServer[0] == '\\' {
			return "", p
		}
		secondSep := strings.IndexByte(afterServer, '\\')
		if secondSep < 0 {
			return `\\` + trimmed[:firstSep] + `\` + afterServer, ""
		}
		root := `\\` + trimmed[:firstSep] + `\` + afterServer[:secondSep]
		return root, strings.TrimLeft(afterServer[secondSep+1:], `\`)
	}
	return "", p
}

func cleanWindowsRelativeSegments(p string) string {
	segments := resolveWindowsSegments(p, false)
	if len(segments) == 0 {
		return "."
	}
	return strings.Join(segments, `\`)
}

func resolveWindowsSegments(raw string, rooted bool) []string {
	parts := strings.Split(raw, `\`)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
			} else if !rooted {
				out = append(out, "..")
			}
		default:
			out = append(out, part)
		}
	}
	return out
}

func joinWindowsRemotePath(base string, elems ...string) string {
	all := make([]string, 0, 1+len(elems))
	if strings.TrimSpace(base) != "" {
		all = append(all, strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(base), "/", `\`), `\`))
	}
	for _, elem := range elems {
		trimmed := strings.Trim(strings.ReplaceAll(strings.TrimSpace(elem), "/", `\`), `\`)
		if trimmed != "" {
			all = append(all, trimmed)
		}
	}
	return cleanWindowsRemotePath(strings.Join(all, `\`))
}

// joinRemotePath joins elems onto base in the path syntax of base.
func joinRemotePath(base string, elems ...string) string {
	if isWindowsRemotePath(base) {
		return joinWindowsRemotePath(base, elems...)
	}
	return path.Join(append([]string{base}, elems...)...)
}

// toSFTPPath converts a Windows remote path into the POSIX-rooted slash form
// required by OpenSSH sftp-server and pkg/sftp on Windows (/C:/Users/...).
// Non-Windows paths are returned unchanged.
func toSFTPPath(remotePath string) string {
	trimmed := strings.TrimSpace(remotePath)
	if !isWindowsRemotePath(trimmed) {
		return remotePath
	}
	cleaned := cleanWindowsRemotePath(trimmed)
	slashed := strings.ReplaceAll(cleaned, `\`, "/")
	if strings.HasPrefix(slashed, "//") {
		return slashed
	}
	if strings.HasPrefix(slashed, "/") {
		return slashed
	}
	return "/" + slashed
}

func isSFTPWindowsDriveRoot(dir string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(dir), "/")
	if len(trimmed) == 2 && isASCIIAlpha(trimmed[0]) && trimmed[1] == ':' {
		return true
	}
	return len(trimmed) == 3 && trimmed[0] == '/' && isASCIIAlpha(trimmed[1]) && trimmed[2] == ':'
}

func sshRemoteAgentctlBinaryPath(platform SSHRemotePlatform) string {
	if isWindowsRemotePlatform(platform) {
		return sshRemoteWindowsAgentctlPath
	}
	return sshRemoteAgentctlPath
}

func sshRemoteAgentctlSha256Path(platform SSHRemotePlatform) string {
	if isWindowsRemotePlatform(platform) {
		return sshRemoteWindowsAgentctlSha
	}
	return sshRemoteAgentctlSha256
}

func sshPlatformFromMetadata(md map[string]interface{}, hintPaths ...string) SSHRemotePlatform {
	if raw := strings.TrimSpace(getMetadataString(md, MetadataKeySSHRemotePlatform)); raw != "" {
		if goos, goarch, ok := strings.Cut(raw, "/"); ok {
			normalizedOS := normalizeSSHRemoteOS(goos)
			normalizedArch := normalizeSSHRemoteArch(goarch)
			if normalizedOS != "" {
				return SSHRemotePlatform{GOOS: normalizedOS, GOARCH: normalizedArch}
			}
		}
	}
	for _, hint := range hintPaths {
		if isWindowsRemotePath(hint) {
			return SSHRemotePlatform{GOOS: sshRemoteGOOSWindows, GOARCH: sshRemoteGOARCHAMD64}
		}
	}
	if isWindowsRemotePath(getMetadataString(md, MetadataKeySSHRemoteTaskDir)) ||
		isWindowsRemotePath(getMetadataString(md, MetadataKeySSHRemoteSessionDir)) {
		return SSHRemotePlatform{GOOS: sshRemoteGOOSWindows, GOARCH: sshRemoteGOARCHAMD64}
	}
	return SSHRemotePlatform{}
}

func sshRemotePathsEqual(left, right string, platform SSHRemotePlatform) bool {
	normLeft := normalizeSSHRemotePath(left)
	normRight := normalizeSSHRemotePath(right)
	if isWindowsRemotePlatform(platform) || isWindowsRemotePath(left) || isWindowsRemotePath(right) {
		return strings.EqualFold(normLeft, normRight)
	}
	return normLeft == normRight
}

// quoteWindowsArg quotes arg for CommandLineToArgvW even when it has no
// whitespace, which keeps cmd.exe metacharacters such as & literal.
func quoteWindowsArg(arg string) string {
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for i := 0; i < len(arg); i++ {
		switch arg[i] {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteByte('"')
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat(`\`, backslashes))
				backslashes = 0
			}
			b.WriteByte(arg[i])
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

func gitCredentialHelperCommand(agentctlBin string) string {
	if isWindowsRemotePath(agentctlBin) {
		slashed := strings.ReplaceAll(cleanWindowsRemotePath(agentctlBin), `\`, "/")
		return "!" + shellQuote(slashed) + " git-credential"
	}
	if strings.ContainsAny(agentctlBin, " \t'\"") {
		return "!" + shellQuote(agentctlBin) + " git-credential"
	}
	return "!" + agentctlBin + " git-credential"
}
