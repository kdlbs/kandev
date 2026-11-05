package mcp

import (
	"net/http"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	profile "github.com/kandev/kandev/internal/mcp/profile"
)

// ServeBuiltinStreamableHTTP retains package isolation behind an authenticated route.
func (s *Server) ServeBuiltinStreamableHTTP(id profile.PackageID, w http.ResponseWriter, request *http.Request) {
	path := contract.MCPRootPath
	if id != profile.PackageCore {
		path = contract.MCPPackagePathPrefix + string(id)
	}
	if request.URL.EscapedPath() != path || request.URL.RawQuery != "" || request.URL.ForceQuery {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if id == profile.PackageCore {
		s.ServeStreamableHTTP(w, request)
		return
	}
	s.mu.RLock()
	view := s.builtinPackages[id]
	s.mu.RUnlock()
	if view == nil || view.http == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	compatibleStreamableHTTPHandler(view.http).ServeHTTP(w, request)
}
