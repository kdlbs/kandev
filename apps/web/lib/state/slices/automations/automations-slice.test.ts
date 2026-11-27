import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { Automation } from "@/lib/types/automation";
import { defaultAutomationsState } from "./automations-slice";

function row(workspace: string, id: string): Automation {
  return {
    id,
    name: id,
    workspace_id: workspace,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-10-05T00:00:00Z",
    updated_at: "2026-10-05T00:00:00Z",
    triggers: [],
  };
}

describe("automation list cache compatibility", () => {
  it("keeps sequential mutations in their loaded workspace entries and preserves other state", () => {
    const a = row("a", "a-row");
    const b = row("b", "b-row");
    const store = createAppStore({
      automations: {
        ...defaultAutomationsState.automations,
        items: [b],
        loaded: true,
        triggerTypes: { a: { items: [], loading: true, generation: 7 } },
        byWorkspace: {
          a: { items: [a], loaded: true, loading: false, generation: 3 },
          b: { items: [b], loaded: true, loading: false, generation: 2 },
        },
      },
      automationRuns: { ...defaultAutomationsState.automationRuns, mutationEpoch: { existing: 8 } },
    });
    const before = store.getState();
    const created = row("a", "created-a");
    before.addAutomation(created);
    expect(store.getState().automations.byWorkspace!.a.items).toEqual([created, a]);
    expect(store.getState().automations.items).not.toBe(before.automations.items);
    store.getState().updateAutomation({ ...created, name: "Renamed", enabled: false });
    expect(store.getState().automations.byWorkspace!.a.items[0]).toMatchObject({
      name: "Renamed",
      enabled: false,
    });
    store.getState().removeAutomation(created.id);
    const after = store.getState();
    expect(after.automations.byWorkspace!.a.items).toEqual([a]);
    expect(after.automations.byWorkspace!.b).toEqual(before.automations.byWorkspace!.b);
    expect(after.automations.triggerTypes).toEqual(before.automations.triggerTypes);
    expect(after.automationRuns).toEqual(before.automationRuns);
  });

  it("does not turn a mutation into a partially loaded list", () => {
    const store = createAppStore();
    store.getState().addAutomation(row("unknown", "created"));
    expect(store.getState().automations.byWorkspace).toEqual({});
    expect(store.getState().automations.items[0].id).toBe("created");
  });

  it("supplies workspace defaults for a legacy hydrated flat list", () => {
    const legacy = row("a", "legacy-a");
    const store = createAppStore({
      automations: {
        items: [legacy],
        loaded: true,
        loading: false,
        triggerTypes: {},
      },
    });
    expect(store.getState().automations.byWorkspace).toEqual({});
    expect(store.getState().automations.items).toEqual([legacy]);
  });
});
