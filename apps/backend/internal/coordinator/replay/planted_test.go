package replay

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The planted suite is the harness's proof: a fixed case set, a deterministic
// model that answers from testdata/planted/model.json and candidates with a
// recorded guard result and verdict. It makes no network call.

type plantedCase struct{ ID, Approved, Rejected string }

type plantedRule struct {
	RepeatRejectedIn []string `json:"repeat_rejected_in"`
	DropApprovedIn   []string `json:"drop_approved_in"`
}

type plantedCandidate struct {
	Name      string `json:"name"`
	Guard     string `json:"guard"`
	Verdict   string `json:"verdict"`
	GuardOnly bool   `json:"guardOnly"`
}

func loadJSON(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile("testdata/planted/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func in(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// plantedWorld builds the fixed case set and the stub model for one candidate.
func plantedWorld(t *testing.T, candidate string) *world {
	t.Helper()
	var cases []plantedCase
	loadJSON(t, "cases.json", &cases)
	var model map[string]plantedRule
	loadJSON(t, "model.json", &model)
	w := newWorld()
	for _, c := range cases {
		w.addCase(c.ID, caseProp{c.Approved, DecisionApproved}, caseProp{c.Rejected, DecisionRejected})
	}
	byID := map[string]plantedCase{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	w.renders = Renders{BaselineText: "MARK:baseline", BaselineHash: "planted-baseline", CandidateText: "MARK:" + candidate, CandidateHash: "planted-" + candidate}
	w.answer = func(prompt string) (Reply, error) {
		marker := "baseline"
		if i := strings.Index(prompt, "MARK:"); i >= 0 {
			marker = strings.Fields(prompt[i+5:])[0]
		}
		rule, ok := model[marker]
		if !ok {
			t.Errorf("no model row for marker %q", marker)
		}
		id := strings.Fields(prompt[strings.Index(prompt, "CASE:")+5:])[0]
		c := byID[id]
		var parts []string
		if !in(rule.DropApprovedIn, id) {
			parts = append(parts, `{"kind":"move","target_task_id":"`+c.Approved+`"}`)
		}
		if in(rule.RepeatRejectedIn, id) {
			parts = append(parts, `{"kind":"move","target_task_id":"`+c.Rejected+`"}`)
		}
		return Reply{Text: "[" + strings.Join(parts, ",") + "]", PromptTokens: 50, ResponseTokens: 5}, nil
	}
	return w
}

func TestPlantedCandidates(t *testing.T) {
	var cands []plantedCandidate
	loadJSON(t, "candidates.json", &cands)
	if len(cands) != 5 {
		t.Fatalf("the planted set is four known-bad and one known-good candidate, got %d", len(cands))
	}
	for _, c := range cands {
		t.Run(c.Name, func(t *testing.T) {
			w := plantedWorld(t, c.Name)
			res, err := w.harness().Run(context.Background(), Request{
				CoordinatorID: "c1", Candidate: Candidate{Kind: KindContextDiff, Text: "MARK:" + c.Name}})
			if err != nil {
				t.Fatal(err)
			}
			if res.Guard != c.Guard || res.Verdict != c.Verdict {
				t.Fatalf("%s: guard %s verdict %s (reason %q), recorded %s/%s", c.Name, res.Guard, res.Verdict, res.Reason, c.Guard, c.Verdict)
			}
			if res.HeldOutCompared < MinHeldOut {
				t.Fatalf("held-out compared %d: the fixture must hold at least %d", res.HeldOutCompared, MinHeldOut)
			}
			if c.GuardOnly {
				gain := *res.HeldOutCandidateScore - *res.HeldOutBaselineScore
				if gain < MinGainThousandths {
					t.Fatalf("the guard-only candidate must beat the baseline by %d, gained %d: the fixture drifted", MinGainThousandths, gain)
				}
				// With the guard off the verdict would be an improvement.
				if v, _ := judge(GuardPass, res.HeldOutCompared, *res.HeldOutCandidateScore, *res.HeldOutBaselineScore); v != VerdictImprovement {
					t.Fatalf("with the guard off the candidate would be %s", v)
				}
			}
		})
	}
}

func TestPlantedBaselineReproducesAnApproval(t *testing.T) {
	// The drop-everything candidate flips only because the baseline reproduces approvals.
	w := plantedWorld(t, "drop-all")
	res := mustRun(t, w, Request{CoordinatorID: "c1", Candidate: Candidate{Kind: KindContextDiff, Text: "MARK:drop-all"}})
	if len(res.Flips) != 25 {
		t.Fatalf("flips %d, want one per case", len(res.Flips))
	}
}
