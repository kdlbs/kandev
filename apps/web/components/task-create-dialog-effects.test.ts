import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  useAutoFillTaskNameFromPR,
  useBranchAutoSelectEffect,
  useDefaultSelectionsEffect,
  useWorkflowAgentProfileEffect,
} from "./task-create-dialog-effects";
import type { DialogFormState, StoreSelections } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Workspace } from "@/lib/types/http";
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

type DefaultSelFake = Pick<
  DialogFormState,
  | "agentProfileId"
  | "workflowAgentProfileId"
  | "selectedWorkflowId"
  | "executorId"
  | "executorProfileId"
  | "setAgentProfileId"
  | "setExecutorId"
  | "setExecutorProfileId"
  | "noRepository"
  | "repositories"
>;
function makeDefaultSelFs(overrides: Partial<DefaultSelFake> = {}): DialogFormState {
  return {
    agentProfileId: "",
    workflowAgentProfileId: "",
    selectedWorkflowId: null,
    executorId: "exec-1",
    executorProfileId: "profile-1",
    setAgentProfileId: vi.fn(),
    setExecutorId: vi.fn(),
    setExecutorProfileId: vi.fn(),
    noRepository: false,
    // Multi-repo guard effect inside useDefaultSelectionsEffect reads
    // fs.repositories when executorProfileId is set. Empty list keeps that
    // branch a no-op for these autopick-focused tests.
    repositories: [],
    ...overrides,
  } as unknown as DialogFormState;
}

function makeSel(overrides: Partial<StoreSelections> = {}): StoreSelections {
  return {
    agentProfiles: [],
    compatibleAgentProfiles: [],
    // Default to loaded=true so the bulk of existing-behavior tests don't
    // each have to flip it explicitly. The dedicated deferral test overrides.
    authLoaded: true,
    executors: [],
    workspaceDefaults: null,
    ...overrides,
  };
}

describe("useDefaultSelectionsEffect — executor-aware agent restoration", () => {
  it("restores lastId when it is compatible with the current executor", async () => {
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(cursor.id));
    const fs = makeDefaultSelFs();
    const sel = makeSel({
      agentProfiles: [claude, cursor],
      compatibleAgentProfiles: [cursor],
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setAgentProfileId).toHaveBeenCalledWith(cursor.id));
  });

  it("falls back to first compatible when lastId is incompatible with the current executor (regression for sprites + Claude)", async () => {
    // Real-world scenario: user last opened the dialog under local executor
    // and picked Claude. Now they're switching to Sprites where Claude has
    // no creds wired. Before the fix this restored Claude → noCompatibleAgent
    // flipped true via the "selected agent not in compatible list" branch →
    // dialog showed "No compatible agent profiles" despite Cursor/Codex being
    // wired up.
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    const codex = makeProfile("codex");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs();
    const sel = makeSel({
      agentProfiles: [claude, cursor, codex],
      compatibleAgentProfiles: [cursor, codex],
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setAgentProfileId).toHaveBeenCalledWith(cursor.id));
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);
  });

  it("prefers the workspace default over the first compatible when lastId is incompatible but defId is compatible", async () => {
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    const codex = makeProfile("codex");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs();
    const sel = makeSel({
      agentProfiles: [claude, cursor, codex],
      compatibleAgentProfiles: [cursor, codex],
      workspaceDefaults: { default_agent_profile_id: codex.id } as unknown as Workspace,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setAgentProfileId).toHaveBeenCalledWith(codex.id));
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith(cursor.id);
  });

  it("skips defId when it is incompatible and falls back to first compatible", async () => {
    const claude = makeProfile("claude");
    const gemini = makeProfile("gemini");
    const cursor = makeProfile("cursor");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs();
    const sel = makeSel({
      agentProfiles: [claude, gemini, cursor],
      compatibleAgentProfiles: [cursor],
      // Workspace default points at an incompatible agent — bypass it and
      // fall through to the first compatible one rather than restoring a
      // selection that would itself trip the empty-state gate.
      workspaceDefaults: { default_agent_profile_id: gemini.id } as unknown as Workspace,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setAgentProfileId).toHaveBeenCalledWith(cursor.id));
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith(gemini.id);
  });

  it("does not pick anything when no profile is compatible with the executor", async () => {
    // The dialog renders the "No compatible agent profiles" empty state in
    // this branch — silently picking an incompatible profile would just trip
    // the same gate via the "selected agent not in compatible list" check,
    // producing a worse UX (selector populated, but submit still blocked).
    const claude = makeProfile("claude");
    const gemini = makeProfile("gemini");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs();
    const sel = makeSel({
      agentProfiles: [claude, gemini],
      compatibleAgentProfiles: [],
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    // Give any deferred setter a chance to fire so we'd catch an unwanted call.
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();
  });

  it("does nothing when agentProfileId is already set (user has explicitly picked)", async () => {
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(cursor.id));
    const fs = makeDefaultSelFs({ agentProfileId: claude.id });
    const sel = makeSel({
      agentProfiles: [claude, cursor],
      compatibleAgentProfiles: [cursor],
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();
  });
});

describe("useDefaultSelectionsEffect — auth-spec load race", () => {
  it("defers auto-pick while executorProfileId is empty even when authLoaded (regression for race that restored an incompatible lastId)", async () => {
    // The other half of the executor-aware autopick race: `authLoaded` flips
    // true before `executorProfileId` settles, because the executor
    // auto-select runs as its own useEffect via a Promise.resolve().then()
    // microtask. During that single render `useExecutorProfileCompat`
    // short-circuits to `agentProfiles` (no filter) because
    // `selectedExecutorProfile` is null, so without this gate we'd happily
    // restore an incompatible lastId, then the executor lands milliseconds
    // later, the filter narrows the list, and `noCompatibleAgent` flips true.
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs({ executorProfileId: "" });
    // Mirror the production shape: specs loaded, no executor picked yet, so
    // useExecutorProfileCompat returns agentProfiles unfiltered.
    const selBefore = makeSel({
      agentProfiles: [claude, cursor],
      compatibleAgentProfiles: [claude, cursor],
      authLoaded: true,
      executors: [{ id: "exec-1" } as unknown as StoreSelections["executors"][number]],
    });
    const { rerender } = renderHook(
      ({ sel, fs: fsArg }) => useDefaultSelectionsEffect(fsArg, true, sel, []),
      { initialProps: { sel: selBefore, fs } },
    );
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();

    // Executor finally lands. Compat filter narrows the list. lastId=claude
    // is no longer compatible; effect re-fires and picks the first compatible.
    const fsAfter = makeDefaultSelFs({ executorProfileId: "profile-1" });
    const selAfter = makeSel({
      agentProfiles: [claude, cursor],
      compatibleAgentProfiles: [cursor],
      authLoaded: true,
      executors: [{ id: "exec-1" } as unknown as StoreSelections["executors"][number]],
    });
    rerender({ sel: selAfter, fs: fsAfter });
    await waitFor(() => expect(fsAfter.setAgentProfileId).toHaveBeenCalledWith(cursor.id));
    expect(fsAfter.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);
  });

  it("defers auto-pick until the remote-auth catalog has loaded (regression for race that restored an incompatible lastId)", async () => {
    // Race the bundled effect hit in production: dialog opens, lastId=claude.
    // While authLoaded=false, useExecutorProfileCompat short-circuits to
    // `compatibleAgentProfiles = agentProfiles` (full list) → effect happily
    // restores claude → specs land → filter kicks in → claude drops out of
    // compatibleAgentProfiles → noCompatibleAgent flips true via the
    // "selected-not-in-compatible" branch → "No compatible agent profiles"
    // empty state shows even though Codex / OpenCode / Cursor are right there.
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    const codex = makeProfile("codex");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(claude.id));
    const fs = makeDefaultSelFs();
    // Phase 1: specs not yet loaded — compatibleAgentProfiles is the full
    // list because the executor-compat hook returns agentProfiles when
    // !authLoaded. The effect MUST NOT auto-pick yet.
    const selBefore = makeSel({
      agentProfiles: [claude, cursor, codex],
      compatibleAgentProfiles: [claude, cursor, codex],
      authLoaded: false,
    });
    const { rerender } = renderHook(({ sel }) => useDefaultSelectionsEffect(fs, true, sel, []), {
      initialProps: { sel: selBefore },
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();

    // Phase 2: specs land. compatibleAgentProfiles narrows to the executor's
    // actual valid set. Now the effect re-fires and must skip the
    // incompatible lastId, landing on the first compatible profile.
    const selAfter = makeSel({
      agentProfiles: [claude, cursor, codex],
      compatibleAgentProfiles: [cursor, codex],
      authLoaded: true,
    });
    rerender({ sel: selAfter });
    await waitFor(() => expect(fs.setAgentProfileId).toHaveBeenCalledWith(cursor.id));
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);
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

const PR_1_TITLE = "PR #1: first";

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
    expect(currentName).toBe(PR_1_TITLE);

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
    expect(currentName).toBe(PR_1_TITLE);

    // User edits the title manually.
    currentName = "my edits";
    rerender();

    result.current(2, "second");
    // Setter is still only called once (the initial auto-fill); the manual
    // edit is preserved.
    expect(fs.setTaskName).toHaveBeenCalledTimes(1);
    expect(currentName).toBe("my edits");
  });

  it("clears the sentinel when taskName resets to empty, preventing a re-typed auto-fill from being overwritten", () => {
    // Regression guard for the mounted-across-open/close edge case:
    // after a dialog reset clears taskName, the user manually types the same
    // text that was previously auto-filled. A second PR paste must NOT
    // overwrite it because the sentinel was cleared on the empty transition.
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
    expect(currentName).toBe(PR_1_TITLE);

    // Simulate React flushing the setTaskName state update back into the hook
    // so the ref-mirror effect sees taskName = PR_1_TITLE.
    rerender();

    // Dialog resets taskName to empty (sentinel should clear).
    currentName = "";
    rerender();

    // User re-types the exact previously auto-filled value.
    currentName = PR_1_TITLE;
    rerender();

    // A second paste with a different PR should NOT overwrite.
    result.current(2, "second");
    expect(fs.setTaskName).toHaveBeenCalledTimes(1); // only the first auto-fill
    expect(currentName).toBe(PR_1_TITLE);
  });
});
