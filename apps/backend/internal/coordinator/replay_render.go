package coordinator

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

// ErrReplayNoTarget reports a candidate that names nothing to apply to.
var ErrReplayNoTarget = replay.ErrNoTarget

// ReplayRenderInput is the coordinator state one replay render reads: the
// context, the active orders and the goal section, all from one snapshot.
type ReplayRenderInput struct {
	WorkspaceName string
	WorkspaceID   string
	Name          string
	Context       string
	Orders        []StandingOrder
	GoalSection   string
}

// ReplayRender returns the baseline and candidate instruction renders: the
// production StandingInstructions output without the improvement section,
// with the override applied to the candidate side.
func ReplayRender(in ReplayRenderInput, o replay.Override) (replay.Renders, error) {
	base := renderReplayInstructions(in.Context, in.Orders, in, "")
	ctxText, orders, note := in.Context, in.Orders, ""
	switch o.Kind {
	case replay.KindContextDiff:
		ctxText = o.Text
	case replay.KindStandingOrderAdd:
		orders = append(append([]StandingOrder{}, in.Orders...), StandingOrder{ID: "candidate", Text: o.Text})
	case replay.KindStandingOrderRetir:
		kept := make([]StandingOrder, 0, len(in.Orders))
		for _, ord := range in.Orders {
			if ord.ID != o.TargetID {
				kept = append(kept, ord)
			}
		}
		if len(kept) == len(in.Orders) {
			return replay.Renders{}, ErrReplayNoTarget
		}
		orders = kept
	case replay.KindNoteAdd:
		note = replay.NoteSectionPrefix + o.Text
	default:
		return replay.Renders{}, ErrReplayNoTarget
	}
	cand := renderReplayInstructions(ctxText, orders, in, note)
	if o.Kind == replay.KindStandingOrderAdd {
		cand = strings.Replace(cand, "(added "+time0Format+", id candidate)", "(added candidate, id candidate)", 1)
	}
	return replay.Renders{BaselineText: base, BaselineHash: hashText(base), CandidateText: cand, CandidateHash: hashText(cand)}, nil
}

// time0Format is the date a zero CreatedAt renders as in a standing order line.
const time0Format = "0001-01-01"

func renderReplayInstructions(ctxText string, orders []StandingOrder, in ReplayRenderInput, note string) string {
	sections := []string{StandingOrdersSection(orders), in.GoalSection}
	if note != "" {
		sections = append(sections, note)
	}
	return StandingInstructions(in.WorkspaceName, in.WorkspaceID, in.Name, ctxText, sections...)
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
