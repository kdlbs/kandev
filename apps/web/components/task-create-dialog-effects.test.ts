import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useWorkflowAgentProfileEffect } from "./task-create-dialog-effects";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";
import { STORAGE_KEYS } from "@/lib/settings/constants";

// Minimal fake of DialogFormState - the hook destructures only three fields,
// so the rest can be undefined behind an `as` cast and never read.
type Fake = Pick<
  DialogFormState,
  "selectedWorkflowId" | "setAgentProfileId" | "setWorkflowAgentProfileId"
>;
function makeFs(overrides: Partial<Fake> = {}): DialogFormState {
  return {
    selectedWorkflowId: null,
    setAgentProfileId: vi.fn(),
    setWorkflowAgentProfileId: vi.fn(),
    ...overrides,
  } as unknown as DialogFormState;
}

function makeProfile(id: string): AgentProfileOption {
  return {
    id,
    label: `agent • ${id}`,
    agent_id: `agent-${id}`,
    agent_name: "agent",
    cli_passthrough: false,
  };
}

beforeEach(() => {
  localStorage.clear();
});

describe("useWorkflowAgentProfileEffect", () => {
  it("clears workflowAgentProfileId and leaves agentProfileId alone when no workflow is selected", () => {
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("stale-id"));
    const fs = makeFs({ selectedWorkflowId: null });

    renderHook(() => useWorkflowAgentProfileEffect(fs, [], [makeProfile("real-id")]));

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("");
    // No workflow → effect must early-return before touching agentProfileId,
    // even if localStorage holds a value (stale or otherwise).
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();
  });

  it("locks to workflow.agent_profile_id when the profile exists", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1", agent_profile_id: "real-id" }];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, [makeProfile("real-id")]));

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("real-id");
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("real-id");
  });

  it("locks the selector but does NOT set agentProfileId when workflow's profile is missing", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1", agent_profile_id: "missing-id" }];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, [makeProfile("real-id")]));

    // The lock still applies (otherwise the user could pick an alternate
    // profile and submit a workflow-locked task with the wrong agent).
    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("missing-id");
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith("missing-id");
  });

  it("restores last-used agentProfileId when the workflow has no override and the id is still valid", () => {
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("real-id"));
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, [makeProfile("real-id")]));

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("");
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("real-id");
  });

  it("does NOT restore a stale localStorage agentProfileId that is not in agentProfiles", () => {
    // Regression guard: a clean DB mints fresh UUIDs for the same agents, so
    // the previous run's lastAgentProfileId no longer matches any profile.
    // Writing it back here poisons the dialog into "No compatible agent
    // profiles" because useDefaultSelectionsEffect skips when agentProfileId
    // is already truthy.
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("stale-id"));
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, [makeProfile("real-id")]));

    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith("stale-id");
  });

  it("clears agentProfileId when the workflow has no override and there is no last-used id", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, [makeProfile("real-id")]));

    // Empty string lets useDefaultSelectionsEffect take over the fallback
    // chain (workspace default → first profile).
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("");
  });
});
