package storeconformance

import (
	"fmt"

	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	orchestrationstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	runsstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

const (
	conformanceAgentID = "conformance-agent"
)

const (
	conformanceValue = "conformance"
)

func runsSchema(s testconformance.ScenarioContext) error {
	return runsstore.NewWithDB(s.DB, s.DB).Migrate()
}

func orchestrationSchema(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	if err := agentSettingsSchema(s); err != nil {
		return err
	}
	return orchestrationstore.New(s.DB, s.DB).Migrate()
}

func runsAction() apiAction {
	action := standardAction(reflectedSpec{
		name: "runs",
		populate: func(record any, id string) {
			run := record.(*runsmodels.Run)
			run.ID, run.AgentProfileID, run.Reason, run.Status = id, conformanceAgentID, conformanceValue, "queued"
		},
		factory: func(s testconformance.ScenarioContext) (any, error) {
			return runsstore.NewWithDB(s.DB, s.DB), nil
		},
		createMethod: "CreateRun", readMethod: "GetRunByID",
		updateMethod: "UpdateRunOutputSummary", deleteMethod: "CancelRun",
		updateArgs: func(id string, _ any) []any { return []any{id, "verified result", ""} },
		deleteArgs: func(id string) []any { return []any{id, "conformance cleanup"} },
		updateAssertion: func(_ any, after any, _ string) error {
			run := after.(*runsmodels.Run)
			if run.OutputSummary != "verified result" || run.ContinuationScope == "" || run.RequestedAt.IsZero() {
				return fmt.Errorf("run lost its output, continuation scope, or admission timestamp")
			}
			return nil
		},
	})
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		run, err := runsstore.NewWithDB(s.DB, s.DB).GetRunByID(s.Context, id)
		if err != nil {
			return err
		}
		if run.Status != "cancelled" || run.CancelReason == nil || *run.CancelReason != "conformance cleanup" {
			return fmt.Errorf("cancel did not retire the queued run")
		}
		return nil
	}
	return action
}

func orchestrationMemoryAction() apiAction {
	store := func(s testconformance.ScenarioContext) *orchestrationstore.Repository {
		return orchestrationstore.New(s.DB, s.DB)
	}
	action := apiAction{
		name: "orchestration_memory",
		create: func(s testconformance.ScenarioContext, id string) (any, error) {
			memory := &orchestrationmodels.AgentMemory{
				ID: id, AgentProfileID: conformanceAgentID, Layer: "working", Key: id,
				Content: "initial context", Metadata: "{}",
			}
			return memory, store(s).UpsertAgentMemory(s.Context, memory)
		},
		read: func(s testconformance.ScenarioContext, id string) (any, error) {
			memory, err := store(s).GetAgentMemory(s.Context, conformanceAgentID, "working", id)
			if err != nil {
				return nil, err
			}
			rows, err := store(s).AssistantMemoryPage(s.Context, conformanceAgentID, "conformance-owner", "workspace", "", "", 50)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.ID == id {
					return memory, nil
				}
			}
			return nil, fmt.Errorf("owner memory page omitted eligible legacy workspace memory")
		},
		update: func(s testconformance.ScenarioContext, _ string, record any) error {
			memory := *record.(*orchestrationmodels.AgentMemory)
			memory.Content = "updated context"
			return store(s).UpsertAgentMemory(s.Context, &memory)
		},
		assertUpdated: func(_ any, after any, _ string) error {
			if after.(*orchestrationmodels.AgentMemory).Content != "updated context" {
				return fmt.Errorf("memory content was not updated")
			}
			return nil
		},
		delete: func(s testconformance.ScenarioContext, id string) error {
			return store(s).DeleteAgentMemory(s.Context, id)
		},
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, record any) error {
		memory := *record.(*orchestrationmodels.AgentMemory)
		memory.ID = id + "-duplicate"
		memory.Content = "replacement context"
		if err := store(s).UpsertAgentMemory(s.Context, &memory); err != nil {
			return err
		}
		actual, err := store(s).GetAgentMemory(s.Context, memory.AgentProfileID, memory.Layer, memory.Key)
		if err != nil {
			return err
		}
		if actual.ID != id || actual.Content != memory.Content || actual.Revision != 2 {
			return fmt.Errorf("memory conflict did not preserve identity and advance its revision")
		}
		return nil
	}
	return action
}
