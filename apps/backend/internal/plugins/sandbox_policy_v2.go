package plugins

import (
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kdlbs/kandev/pluginsdk"
)

func validSandboxPolicyV2(value *pluginsdk.SandboxPolicyV2) bool {
	if value == nil || value.SchemaVersion != 2 || !validSandboxPermissionsV2(value.StaticGrants) || !validSandboxPermissionsV2(value.ApprovalEnvelope) || len(value.ResourcePolicies) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, resource := range value.ResourcePolicies {
		if resource == nil || !sandboxDigestV2(resource.Digest) || seen[resource.Digest] || !validSandboxPermissionsV2(resource.Permissions) {
			return false
		}
		seen[resource.Digest] = true
	}
	for _, permissions := range []*pluginsdk.SandboxPermissionsV2{value.StaticGrants, value.ApprovalEnvelope} {
		for _, command := range permissions.Commands {
			if !seen[command.ResourcePolicyDigest] {
				return false
			}
		}
	}
	return sandboxResourceGraphV2(value.ResourcePolicies)
}

func sandboxResourceGraphV2(resources []*pluginsdk.SandboxResourcePolicyV2) bool {
	graph := map[string]*pluginsdk.SandboxPermissionsV2{}
	for _, resource := range resources {
		graph[resource.Digest] = resource.Permissions
	}
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string, int) bool
	visit = func(key string, depth int) bool {
		if done[key] {
			return true
		}
		permissions, ok := graph[key]
		if !ok || active[key] || depth > 8 {
			return false
		}
		active[key] = true
		for _, command := range permissions.Commands {
			if !visit(command.ResourcePolicyDigest, depth+1) {
				return false
			}
		}
		active[key] = false
		done[key] = true
		return true
	}
	for key := range graph {
		if !visit(key, 0) {
			return false
		}
	}
	return true
}

func validSandboxPermissionsV2(value *pluginsdk.SandboxPermissionsV2) bool {
	if !sandboxPermissionBoundsV2(value) {
		return false
	}
	for _, grant := range value.Filesystem {
		if !validSandboxFilesystemV2(grant) {
			return false
		}
	}
	for _, grant := range value.Tcp {
		if !validSandboxTCPV2(grant) {
			return false
		}
	}
	for _, grant := range value.Commands {
		if !validSandboxCommandV2(grant) {
			return false
		}
	}
	for _, grant := range value.HttpEndpoints {
		if !validSandboxEndpointV2(grant) {
			return false
		}
	}
	return true
}

func sandboxPermissionBoundsV2(value *pluginsdk.SandboxPermissionsV2) bool {
	if value == nil || len(value.Filesystem) > 256 || len(value.Tcp) > 256 || len(value.Commands) > 128 || len(value.HttpEndpoints) > 256 || !sandboxUniqueTextV2(value.EnvironmentKeys, 256, 128) {
		return false
	}
	for _, key := range value.EnvironmentKeys {
		if strings.Trim(key, "ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789") != "" || key[0] >= '0' && key[0] <= '9' {
			return false
		}
	}
	return true
}

func validSandboxFilesystemV2(grant *pluginsdk.SandboxFilesystemPermissionV2) bool {
	if grant == nil || (grant.Path == "") == (grant.Reference == "") {
		return false
	}
	if grant.Path != "" {
		return sandboxAbsolutePathV2(grant.Path)
	}
	return slices.Contains([]string{sandboxReferenceWorkDir, sandboxReferenceAgentState}, grant.Reference)
}

func validSandboxOperationV2(value *pluginsdk.SandboxOperationV2) bool {
	if value == nil {
		return false
	}
	switch operation := value.Value.(type) {
	case *pluginsdk.SandboxOperationV2_Filesystem:
		return operation.Filesystem != nil && sandboxAbsolutePathV2(operation.Filesystem.Path) && slices.Contains([]string{sandboxAccessRead, sandboxV2Write, sandboxV2ReadWrite}, operation.Filesystem.Access)
	case *pluginsdk.SandboxOperationV2_Tcp:
		return validSandboxTCPV2(operation.Tcp)
	case *pluginsdk.SandboxOperationV2_Command:
		return validSandboxCommandV2(operation.Command)
	case *pluginsdk.SandboxOperationV2_HttpEndpoint:
		return validSandboxEndpointV2(operation.HttpEndpoint)
	default:
		return false
	}
}

func sandboxAbsolutePathV2(value string) bool {
	return validSandboxText(value, 4096) && path.IsAbs(value) && path.Clean(value) == value && !strings.Contains(value, "\\")
}

func validSandboxTCPV2(value *pluginsdk.SandboxTCPDestinationV2) bool {
	return value != nil && sandboxHostV2(value.Host) && value.Port > 0 && value.Port <= 65535 && value.Transport == sandboxTransportTCP
}

func sandboxHostV2(host string) bool {
	if !validSandboxText(host, 253) || strings.ToLower(host) != host {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return false
		}
	}
	return true
}

func validSandboxCommandV2(value *pluginsdk.SandboxCommandInvocationV2) bool {
	if value == nil || !validSandboxText(value.CommandId, 128) || !sandboxAbsolutePathV2(value.ExecutablePath) || !sandboxDigestV2(value.ExecutableSha256) || !sandboxDigestV2(value.ResourcePolicyDigest) || !validSandboxText(value.Caller, 128) || !slices.Contains([]string{sandboxReferenceWorkDir, sandboxReferenceAgentState}, value.WorkDirReference) || len(value.Argv) > 128 {
		return false
	}
	for _, arg := range value.Argv {
		if len(arg) > 4096 || !utf8.ValidString(arg) || strings.ContainsFunc(arg, unicode.IsControl) {
			return false
		}
	}
	return true
}

func validSandboxEndpointV2(value *pluginsdk.SandboxHTTPEndpointV2) bool {
	if value == nil || !validSandboxText(value.RouteId, 128) || !slices.Contains([]string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}, value.Method) {
		return false
	}
	return validSandboxOriginV2(value.Origin) && validSandboxHTTPPathV2(value.Path, value.Query)
}

func validSandboxOriginV2(value string) bool {
	if !validSandboxText(value, 2048) {
		return false
	}
	origin, err := url.Parse(value)
	if err != nil || !sandboxOriginShapeV2(origin) {
		return false
	}
	if origin.Port() == "" {
		return true
	}
	port, err := strconv.Atoi(origin.Port())
	return err == nil && port > 0 && port <= 65535 && strconv.Itoa(port) == origin.Port()
}

func validSandboxHTTPPathV2(target, rawQuery string) bool {
	if !sandboxAbsolutePathV2(target) || strings.ContainsAny(target, "%?#") || len(rawQuery) > 4096 {
		return false
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil || query.Encode() != rawQuery {
		return false
	}
	for _, values := range query {
		if len(values) != 1 {
			return false
		}
	}
	return true
}

func sandboxDigestV2(value string) bool {
	digest, err := hex.DecodeString(value)
	return err == nil && len(digest) == 32 && strings.ToLower(value) == value
}

func sandboxOriginShapeV2(origin *url.URL) bool {
	return origin.Scheme == sandboxV2HTTPS && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == "" && origin.Opaque == "" && !origin.ForceQuery && !strings.HasSuffix(origin.Host, ":") && sandboxHostV2(origin.Hostname())
}
