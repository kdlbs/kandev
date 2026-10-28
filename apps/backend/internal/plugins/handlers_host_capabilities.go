package plugins

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/plugins/hostcaps"
)

type hostCapabilityStatus struct {
	Name          string   `json:"name"`
	MinVersion    int      `json:"min_version"`
	MaxVersion    int      `json:"max_version"`
	IntroducedIn  string   `json:"introduced_in"`
	Deprecated    bool     `json:"deprecated"`
	DeprecatedIn  string   `json:"deprecated_in,omitempty"`
	RetirementIn  string   `json:"retirement_in,omitempty"`
	Replacement   string   `json:"replacement,omitempty"`
	RequiresNames []string `json:"requires,omitempty"`
}

// hostCapabilities serves GET /api/plugins/host-capabilities: the registry of
// capability names, supported ranges, and deprecation state, for administrators.
func (c *Controller) hostCapabilities(ctx *gin.Context) {
	entries := hostcaps.Registry()
	rows := make([]hostCapabilityStatus, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, hostCapabilityStatus{
			Name: entry.Name, MinVersion: entry.Min, MaxVersion: entry.Max, IntroducedIn: entry.IntroducedIn,
			Deprecated: entry.DeprecatedIn != "", DeprecatedIn: entry.DeprecatedIn, RetirementIn: entry.RetireIn,
			Replacement: entry.Replacement, RequiresNames: entry.Requires,
		})
	}
	ctx.JSON(http.StatusOK, gin.H{"capabilities": rows})
}
