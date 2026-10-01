package wire

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/hostutility"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/shared"
)

var testWriter *sqlx.DB

func openStore(t *testing.T) *coordinator.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.db")
	w, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	r, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	writer := sqlx.NewDb(w, "sqlite3")
	s, err := coordinator.NewStore(writer, sqlx.NewDb(r, "sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	testWriter = writer
	return s
}

func seedCoordinator(t *testing.T, s *coordinator.Store, ceiling *int64) *coordinator.Coordinator {
	t.Helper()
	c := &coordinator.Coordinator{ID: "c1", WorkspaceID: "w1", Name: "co", AgentProfileID: "prof", Context: "ctx"}
	if err := s.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if ceiling != nil {
		if _, err := testWriter.Exec(`UPDATE coordinators SET cost_ceiling_subcents = ? WHERE id = ?`, *ceiling, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

type profileReader struct {
	p   *settingsmodels.AgentProfile
	err error
}

func (r profileReader) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return r.p, r.err
}

func TestProfiles_CarriesSafetyFields(t *testing.T) {
	s := openStore(t)
	seedCoordinator(t, s, nil)
	prof := &settingsmodels.AgentProfile{ID: "prof", Model: "m", AutoApprove: true, CommandPrefix: "sandbox",
		CLIFlags: []settingsmodels.CLIFlag{{Flag: "--on", Enabled: true}, {Flag: "--off"}}}
	got, err := Profiles{Store: s, Profiles: profileReader{p: prof}}.Resolve(t.Context(), "c1")
	if err != nil || got.ID != "prof" || got.Model != "m" || !got.AutoApprove || got.Prefix != "sandbox" ||
		len(got.EnabledFlags) != 1 || got.EnabledFlags[0] != "--on" {
		t.Fatalf("profile = %+v %v", got, err)
	}
	for name, p := range map[string]Profiles{
		"unknown coordinator": {Store: s, Profiles: profileReader{p: prof}},
		"read error":          {Store: s, Profiles: profileReader{err: errors.New("boom")}},
		"missing profile":     {Store: s, Profiles: profileReader{}},
	} {
		id := "c1"
		if name == "unknown coordinator" {
			id = "nope"
		}
		if _, err := p.Resolve(t.Context(), id); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

type runner struct {
	res *hostutility.PromptResult
	err error
}

func (r runner) ExecuteProfilePrompt(context.Context, string, string) (*hostutility.PromptResult, error) {
	return r.res, r.err
}

func TestPrompts(t *testing.T) {
	got, err := Prompts{Runner: runner{res: &hostutility.PromptResult{Response: "[]", PromptTokens: 7, ResponseTokens: 2, Model: "ignored"}}}.Run(t.Context(), "p", "x")
	if err != nil || got != (replay.Reply{Text: "[]", PromptTokens: 7, ResponseTokens: 2}) {
		t.Fatalf("reply = %+v %v", got, err)
	}
	if _, err := (Prompts{Runner: runner{err: errors.New("down")}}).Run(t.Context(), "p", "x"); err == nil {
		t.Fatal("runner error was swallowed")
	}
	if _, err := (Prompts{Runner: runner{}}).Run(t.Context(), "p", "x"); err == nil {
		t.Fatal("nil result was accepted")
	}
}

type priceSource struct {
	p  shared.ModelPricing
	ok bool
}

func (s priceSource) LookupForModelWithVersion(context.Context, string) (shared.ModelPricing, string, bool) {
	return s.p, "v", s.ok
}

func TestPrices(t *testing.T) {
	p, ok, err := Prices{Source: priceSource{p: shared.ModelPricing{InputPerMillion: 3, OutputPerMillion: 15}, ok: true}}.Lookup(t.Context(), "m")
	if err != nil || !ok || p.InputPerMillion != 3 || p.OutputPerMillion != 15 {
		t.Fatalf("price = %+v %v %v", p, ok, err)
	}
	if _, ok, err := (Prices{Source: priceSource{}}).Lookup(t.Context(), "m"); ok || err != nil {
		t.Fatalf("unknown model = %v %v", ok, err)
	}
}

type spendSource struct {
	r   coordinator.SpendReading
	err error
}

func (s spendSource) Spend(context.Context, *coordinator.Coordinator, time.Time) (coordinator.SpendReading, error) {
	return s.r, s.err
}

func TestSpend(t *testing.T) {
	s := openStore(t)
	ceiling := int64(900)
	seedCoordinator(t, s, &ceiling)
	got, err := Spend{Store: s, Source: spendSource{r: coordinator.SpendReading{WindowSubcents: 40, Measurable: true}}}.Reading(t.Context(), "c1", time.Now())
	if err != nil || got.WindowSubcents != 40 || !got.Measurable || got.CeilingSubcents == nil || *got.CeilingSubcents != 900 {
		t.Fatalf("reading = %+v %v", got, err)
	}
	deg, err := Spend{Store: s, Source: spendSource{r: coordinator.SpendReading{WindowSubcents: 40, Degraded: true}}}.Reading(t.Context(), "c1", time.Now())
	if err != nil || deg.Measurable {
		t.Fatalf("degraded reading = %+v %v, want unmeasurable", deg, err)
	}
	if _, err := (Spend{Store: s, Source: spendSource{err: errors.New("x")}}).Reading(t.Context(), "c1", time.Now()); err == nil {
		t.Fatal("source error was swallowed")
	}
	if _, err := (Spend{Store: s, Source: spendSource{}}).Reading(t.Context(), "nope", time.Now()); err == nil {
		t.Fatal("unknown coordinator was accepted")
	}
}

type goals struct {
	text string
	err  error
}

func (g goals) GoalInstructionSection(context.Context, string) (string, error) { return g.text, g.err }

func TestInstructions(t *testing.T) {
	s := openStore(t)
	seedCoordinator(t, s, nil)
	in := Instructions{Store: s, Source: goals{text: "GOAL"}, WorkspaceName: func(context.Context, string) string { return "Work" }}
	r, err := in.Render(t.Context(), "c1", replay.Override{Kind: replay.KindContextDiff, Text: "other"})
	if err != nil || !strings.Contains(r.BaselineText, "ctx") || !strings.Contains(r.BaselineText, "GOAL") ||
		!strings.Contains(r.BaselineText, `"Work"`) || !strings.Contains(r.CandidateText, "other") {
		t.Fatalf("render = %v\n%s", err, r.BaselineText)
	}
	if _, err := in.Render(t.Context(), "c1", replay.Override{Kind: replay.KindNoteUpdate}); !errors.Is(err, replay.ErrNoTarget) {
		t.Fatalf("note_update err = %v", err)
	}
	if _, err := in.Render(t.Context(), "nope", replay.Override{Kind: replay.KindNoteAdd}); err == nil || errors.Is(err, replay.ErrNoTarget) {
		t.Fatalf("unknown coordinator err = %v", err)
	}
	bad := Instructions{Store: s, Source: goals{err: errors.New("goal")}}
	if _, err := bad.Render(t.Context(), "c1", replay.Override{Kind: replay.KindNoteAdd}); err == nil || errors.Is(err, replay.ErrNoTarget) {
		t.Fatalf("goal read error = %v", err)
	}
}
