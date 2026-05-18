// Package info serves GET /api/v1/system/info — a small read-only payload
// describing the running binary (version, build commit, build time) and the
// Go runtime (version, OS, arch). Powers the System -> About page.
package info

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Response is the JSON payload returned by GET /api/v1/system/info.
type Response struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Service composes the static build info; instantiated once at boot from
// cmd/kandev (which owns the ldflag-injected variables) and reused per
// request.
type Service struct {
	Version   string
	Commit    string
	BuildTime string
}

// NewService constructs a Service from the ldflag-injected build values.
func NewService(version, commit, buildTime string) *Service {
	return &Service{Version: version, Commit: commit, BuildTime: buildTime}
}

// Info renders the response payload.
func (s *Service) Info() Response {
	return Response{
		Version:   s.Version,
		Commit:    s.Commit,
		BuildTime: s.BuildTime,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// Handler returns the gin handler for GET /api/v1/system/info.
func Handler(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.Info())
	}
}
