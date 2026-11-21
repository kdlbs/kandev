package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notesProbe struct {
	prepared []string
	accepted []string
	section  string
}

func (p *notesProbe) delivery() CoordinatorNotesDelivery {
	return CoordinatorNotesDelivery{
		Prepare: func(_ context.Context, coordinatorID, turnID string) string {
			p.prepared = append(p.prepared, coordinatorID+"/"+turnID)
			return p.section
		},
		Accepted: func(_ context.Context, coordinatorID, turnID string) {
			p.accepted = append(p.accepted, coordinatorID+"/"+turnID)
		},
	}
}

func coordinatorTask(meta map[string]interface{}) *models.Task {
	return &models.Task{ID: "task", Origin: models.TaskOriginCoordinator, Metadata: meta}
}

func TestWithCoordinatorNotes(t *testing.T) {
	ctx := context.Background()
	section := sysprompt.Wrap("NOTES")
	conv := coordinatorTask(map[string]interface{}{models.MetaKeyCoordinatorID: "c1"})

	t.Run("later turn prepends the section and records acceptance", func(t *testing.T) {
		p := &notesProbe{section: section}
		svc := &Service{coordinatorNotes: p.delivery()}
		got, accepted := svc.withCoordinatorNotes(ctx, conv, "turn-1", "hello", false)
		assert.Equal(t, section+"\n\nhello", got)
		assert.Equal(t, []string{"c1/turn-1"}, p.prepared)
		assert.Empty(t, p.accepted, "acceptance is recorded only when the prompt is accepted")
		accepted()
		assert.Equal(t, []string{"c1/turn-1"}, p.accepted)
	})

	t.Run("first turn places the section after the standing block", func(t *testing.T) {
		p := &notesProbe{section: section}
		svc := &Service{coordinatorNotes: p.delivery()}
		prompt := sysprompt.Wrap("STANDING") + "\n\nhello"
		got, _ := svc.withCoordinatorNotes(ctx, conv, "turn-1", prompt, true)
		assert.Equal(t, sysprompt.Wrap("STANDING")+"\n\n"+section+"\n\nhello", got)
	})

	t.Run("first turn without a standing block prepends", func(t *testing.T) {
		p := &notesProbe{section: section}
		svc := &Service{coordinatorNotes: p.delivery()}
		got, _ := svc.withCoordinatorNotes(ctx, conv, "turn-1", "hello", true)
		assert.Equal(t, section+"\n\nhello", got)
	})

	t.Run("no section leaves the prompt and records nothing", func(t *testing.T) {
		p := &notesProbe{}
		svc := &Service{coordinatorNotes: p.delivery()}
		got, accepted := svc.withCoordinatorNotes(ctx, conv, "turn-1", "hello", false)
		assert.Equal(t, "hello", got)
		assert.Nil(t, accepted)
	})

	t.Run("excluded tasks are never prepared", func(t *testing.T) {
		dream := coordinatorTask(map[string]interface{}{
			models.MetaKeyCoordinatorID:      "c1",
			models.MetaKeyCoordinatorPurpose: models.CoordinatorPurposeDream,
		})
		plain := &models.Task{ID: "t", Metadata: map[string]interface{}{models.MetaKeyCoordinatorID: "c1"}}
		cases := map[string]struct {
			task   *models.Task
			turn   string
			prompt string
		}{
			"dream episode":     {dream, "turn-1", "hello"},
			"no coordinator id": {coordinatorTask(nil), "turn-1", "hello"},
			"not a coordinator": {plain, "turn-1", "hello"},
			"nil task":          {nil, "turn-1", "hello"},
			"no turn identity":  {conv, "", "hello"},
			"empty prompt":      {conv, "turn-1", ""},
		}
		for name, tc := range cases {
			p := &notesProbe{section: section}
			svc := &Service{coordinatorNotes: p.delivery()}
			got, accepted := svc.withCoordinatorNotes(ctx, tc.task, tc.turn, tc.prompt, false)
			assert.Equal(t, tc.prompt, got, name)
			assert.Nil(t, accepted, name)
			assert.Empty(t, p.prepared, name)
		}
	})

	t.Run("unwired seam is a no-op", func(t *testing.T) {
		got, accepted := (&Service{}).withCoordinatorNotes(ctx, conv, "turn-1", "hello", false)
		assert.Equal(t, "hello", got)
		assert.Nil(t, accepted)
	})

	t.Run("the section stays hidden from the transcript", func(t *testing.T) {
		p := &notesProbe{section: section}
		svc := &Service{coordinatorNotes: p.delivery()}
		got, _ := svc.withCoordinatorNotes(ctx, conv, "turn-1", "hello", false)
		assert.Equal(t, "hello", sysprompt.StripSystemContent(got))
	})
}

func TestAttachPromptNotesChainsAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	task := coordinatorTask(map[string]interface{}{models.MetaKeyCoordinatorID: "c1"})
	task.WorkspaceID, task.CreatedAt, task.UpdatedAt = "workspace", time.Now().UTC(), time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, task))
	p := &notesProbe{section: sysprompt.Wrap("NOTES")}
	svc := &Service{repo: repo, coordinatorNotes: p.delivery()}
	var order []string
	dispatched := func() { order = append(order, "dispatched") }
	session := &models.TaskSession{ID: "s"}

	prompt, onDispatched := svc.attachPromptNotes(ctx, "task", session, "turn-1", "hello", promptTaskOptions{}, dispatched)
	assert.Contains(t, prompt, "NOTES")
	onDispatched()
	assert.Equal(t, []string{"dispatched"}, order)
	assert.Equal(t, []string{"c1/turn-1"}, p.accepted)

	p.prepared, p.accepted = nil, nil
	for name, tc := range map[string]struct {
		session *models.TaskSession
		opts    promptTaskOptions
	}{
		"internal continuation": {session, promptTaskOptions{internalContinuation: true}},
		"passthrough session":   {&models.TaskSession{ID: "s", IsPassthrough: true}, promptTaskOptions{}},
	} {
		got, _ := svc.attachPromptNotes(ctx, "task", tc.session, "turn-2", "hello", tc.opts, dispatched)
		assert.Equal(t, "hello", got, name)
	}
	assert.Empty(t, p.prepared)
}

func TestChainInitialPromptAcceptedCountsOnlyOnAcceptance(t *testing.T) {
	var order []string
	chained := chainInitialPromptAccepted(func(id string) { order = append(order, "prev:"+id) }, func() { order = append(order, "notes") })
	if len(order) != 0 {
		t.Fatalf("chaining must not run notes before acceptance: %v", order)
	}
	chained("exec-1")
	if len(order) != 2 || order[0] != "prev:exec-1" || order[1] != "notes" {
		t.Fatalf("order = %v", order)
	}
	var n int
	chainInitialPromptAccepted(nil, func() { n++ })("x")
	if n != 1 {
		t.Fatalf("nil prev: notes ran %d times", n)
	}
}
