import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  useAutoFillTaskNameFromPR,
  useBranchAutoSelectEffect,
  useWorkflowAgentProfileEffect,
} from "./task-create-dialog-effects";
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

type BranchFake = Pick<
  DialogFormState,
  "githubBranch" | "githubBranches" | "useGitHubUrl" | "setGitHubBranch" | "githubPrHeadBranch"
>;
function makeBranchFs(overrides: Partial<BranchFake> = {}): DialogFormState {
  return {
    githubBranch: "",
    githubBranches: [],
    useGitHubUrl: true,
    setGitHubBranch: vi.fn(),
    githubPrHeadBranch: null,
    ...overrides,
  } as unknown as DialogFormState;
}

describe("useBranchAutoSelectEffect", () => {
  it("selects the PR head branch when it is present in the base repo's branch list", () => {
    const fs = makeBranchFs({
      githubBranches: [
        { name: "main", type: "remote" },
        { name: "feature/x", type: "remote" },
      ],
      githubPrHeadBranch: "feature/x",
    });
    renderHook(() => useBranchAutoSelectEffect(fs));
    expect(fs.setGitHubBranch).toHaveBeenCalledWith("feature/x");
  });

  it("still surfaces the PR head branch for fork PRs whose head is NOT in the base repo's branch list", () => {
    // Regression guard for the fork-PR display bug: previously the effect
    // fell through to "main" when the PR head was missing from the base
    // repo's branches, visually contradicting the URL the user just pasted.
    const fs = makeBranchFs({
      githubBranches: [
        { name: "main", type: "remote" },
        { name: "develop", type: "remote" },
      ],
      githubPrHeadBranch: "jira-hosted-path-auth",
    });
    renderHook(() => useBranchAutoSelectEffect(fs));
    expect(fs.setGitHubBranch).toHaveBeenCalledWith("jira-hosted-path-auth");
  });

  it("falls back to main when there is no PR head branch", () => {
    const fs = makeBranchFs({
      githubBranches: [
        { name: "feature/y", type: "remote" },
        { name: "main", type: "remote" },
      ],
    });
    renderHook(() => useBranchAutoSelectEffect(fs));
    expect(fs.setGitHubBranch).toHaveBeenCalledWith("main");
  });

  it("does nothing when useGitHubUrl is false", () => {
    const fs = makeBranchFs({
      useGitHubUrl: false,
      githubBranches: [{ name: "main", type: "remote" }],
      githubPrHeadBranch: "feature/x",
    });
    renderHook(() => useBranchAutoSelectEffect(fs));
    expect(fs.setGitHubBranch).not.toHaveBeenCalled();
  });

  it("does nothing once a branch has already been selected (manual override)", () => {
    const fs = makeBranchFs({
      githubBranch: "develop",
      githubBranches: [
        { name: "main", type: "remote" },
        { name: "develop", type: "remote" },
      ],
      githubPrHeadBranch: "feature/x",
    });
    renderHook(() => useBranchAutoSelectEffect(fs));
    expect(fs.setGitHubBranch).not.toHaveBeenCalled();
  });
});

type AutoFillFake = Pick<DialogFormState, "taskName" | "setTaskName" | "setHasTitle">;
function makeAutoFillFs(overrides: Partial<AutoFillFake> = {}): DialogFormState {
  return {
    taskName: "",
    setTaskName: vi.fn(),
    setHasTitle: vi.fn(),
    ...overrides,
  } as unknown as DialogFormState;
}

describe("useAutoFillTaskNameFromPR", () => {
  it("fills the task name with 'PR #N: <title>' when the title is empty", () => {
    const fs = makeAutoFillFs({ taskName: "" });
    const { result } = renderHook(() => useAutoFillTaskNameFromPR(fs));
    result.current(971, "feat/omp-acp-agent");
    expect(fs.setTaskName).toHaveBeenCalledWith("PR #971: feat/omp-acp-agent");
    expect(fs.setHasTitle).toHaveBeenCalledWith(true);
  });

  it("fills when the title contains only whitespace", () => {
    // Regression guard: hasTitle treats whitespace-only as empty, so the
    // auto-fill check must too — otherwise pasting a PR URL after typing a
    // stray space would silently keep the spaces.
    const fs = makeAutoFillFs({ taskName: "   " });
    const { result } = renderHook(() => useAutoFillTaskNameFromPR(fs));
    result.current(7, "fix");
    expect(fs.setTaskName).toHaveBeenCalledWith("PR #7: fix");
  });

  it("does NOT overwrite a title the user typed themselves", () => {
    const fs = makeAutoFillFs({ taskName: "my custom title" });
    const { result } = renderHook(() => useAutoFillTaskNameFromPR(fs));
    result.current(42, "something");
    expect(fs.setTaskName).not.toHaveBeenCalled();
    expect(fs.setHasTitle).not.toHaveBeenCalled();
  });

  it("replaces a previously auto-filled title when a different PR URL is pasted", () => {
    // Re-paste flow: the user pastes PR #1 (autofills "PR #1: foo"), then
    // realizes wrong URL and pastes PR #2. Without the lastAutoFilled-ref
    // tracking, the second paste would see a non-empty title and bail.
    let currentName = "";
    const fs = makeAutoFillFs({
      taskName: currentName,
      setTaskName: vi.fn((v: string) => {
        currentName = v;
      }),
    });
    const { result, rerender } = renderHook(() => {
      // Re-read taskName from the closure on every render so the ref-mirror
      // effect sees the latest value.
      fs.taskName = currentName;
      return useAutoFillTaskNameFromPR(fs);
    });
    result.current(1, "first");
    expect(currentName).toBe("PR #1: first");

    rerender();
    result.current(2, "second");
    expect(currentName).toBe("PR #2: second");
    expect(fs.setTaskName).toHaveBeenCalledTimes(2);
  });

  it("does NOT replace once the user edits the auto-filled title", () => {
    let currentName = "";
    const fs = makeAutoFillFs({
      taskName: currentName,
      setTaskName: vi.fn((v: string) => {
        currentName = v;
      }),
    });
    const { result, rerender } = renderHook(() => {
      fs.taskName = currentName;
      return useAutoFillTaskNameFromPR(fs);
    });
    result.current(1, "first");
    expect(currentName).toBe("PR #1: first");

    // User edits the title manually.
    currentName = "my edits";
    rerender();

    result.current(2, "second");
    // Setter is still only called once (the initial auto-fill); the manual
    // edit is preserved.
    expect(fs.setTaskName).toHaveBeenCalledTimes(1);
    expect(currentName).toBe("my edits");
  });
});
