import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  useDefaultSelectionsEffect,
  useWorkflowAgentProfileEffect,
  useWorkflowStepsEffect,
} from "./task-create-dialog-effects";
import { decideAgentProfileAutopick } from "./task-create-dialog-autopick";
import type { DialogFormState, StoreSelections } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Workspace } from "@/lib/types/http";
import { STORAGE_KEYS } from "@/lib/settings/constants";

const listWorkflowStepsMock = vi.fn();

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: (...args: unknown[]) => listWorkflowStepsMock(...args),
}));

// Minimal fake of DialogFormState - the hook destructures only three fields,
// so the rest can be undefined behind an `as` cast and never read.
type Fake = Pick<
  DialogFormState,
  | "agentProfileId"
  | "workflowAgentProfileId"
  | "selectedWorkflowId"
  | "executorProfileId"
  | "setAgentProfileId"
  | "setWorkflowAgentProfileId"
>;
function makeFs(overrides: Partial<Fake> = {}): DialogFormState {
  return {
    agentProfileId: "",
    workflowAgentProfileId: "",
    selectedWorkflowId: null,
    executorProfileId: "profile-1",
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
  listWorkflowStepsMock.mockReset();
});

function createQueryWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
}

describe("useWorkflowStepsEffect", () => {
  it("loads fetched steps through the workflow-step query", async () => {
    listWorkflowStepsMock.mockResolvedValueOnce({
      steps: [
        { id: "step-2", name: "Second", position: 2, events: { on_entry: [] } },
        { id: "step-1", name: "First", position: 1, events: { on_exit: [] } },
      ],
    });
    const setFetchedSteps = vi.fn();
    const fs = {
      selectedWorkflowId: "workflow-2",
      setFetchedSteps,
    } as unknown as DialogFormState;

    renderHook(() => useWorkflowStepsEffect(fs, "workflow-1"), {
      wrapper: createQueryWrapper(),
    });

    await waitFor(() =>
      expect(setFetchedSteps).toHaveBeenLastCalledWith([
        { id: "step-1", title: "First", events: { on_exit: [] } },
        { id: "step-2", title: "Second", events: { on_entry: [] } },
      ]),
    );
    expect(listWorkflowStepsMock).toHaveBeenCalledWith(
      "workflow-2",
      expect.objectContaining({
        init: expect.objectContaining({ signal: expect.any(AbortSignal) }),
      }),
    );
  });

  it("clears fetched steps when the selected workflow is already current", () => {
    const setFetchedSteps = vi.fn();
    const fs = {
      selectedWorkflowId: "workflow-1",
      setFetchedSteps,
    } as unknown as DialogFormState;

    renderHook(() => useWorkflowStepsEffect(fs, "workflow-1"), {
      wrapper: createQueryWrapper(),
    });

    expect(setFetchedSteps).toHaveBeenCalledWith(null);
    expect(listWorkflowStepsMock).not.toHaveBeenCalled();
  });
});

describe("useWorkflowAgentProfileEffect", () => {
  it("clears workflowAgentProfileId and leaves agentProfileId alone when no workflow is selected", () => {
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("stale-id"));
    const fs = makeFs({ selectedWorkflowId: null });

    renderHook(() =>
      useWorkflowAgentProfileEffect(fs, [], [makeProfile("real-id")], [makeProfile("real-id")]),
    );

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("");
    // No workflow → effect must early-return before touching agentProfileId,
    // even if localStorage holds a value (stale or otherwise).
    expect(fs.setAgentProfileId).not.toHaveBeenCalled();
  });

  it("locks to workflow.agent_profile_id when the profile exists", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1", agent_profile_id: "real-id" }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile("real-id")],
        [makeProfile("real-id")],
      ),
    );

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("real-id");
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("real-id");
  });

  it("locks the selector but does NOT set agentProfileId when workflow's profile is missing", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1", agent_profile_id: "missing-id" }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile("real-id")],
        [makeProfile("real-id")],
      ),
    );

    // The lock still applies (otherwise the user could pick an alternate
    // profile and submit a workflow-locked task with the wrong agent).
    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("missing-id");
    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith("missing-id");
  });

  it("restores last-used agentProfileId when the workflow has no override and the id is still valid", () => {
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("real-id"));
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile("real-id")],
        [makeProfile("real-id")],
      ),
    );

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

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile("real-id")],
        [makeProfile("real-id")],
      ),
    );

    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith("stale-id");
  });

  it("does NOT restore a lastId that exists but is incompatible with the current executor", () => {
    // Regression for #1075 (CodeRabbit): if lastId names a profile present in
    // `agentProfiles` but absent from `compatibleAgentProfiles` (e.g. user
    // switched from a local executor to a remote one without the matching
    // credential), workflow-unlock used to restore it anyway, which made
    // useDefaultSelectionsEffect early-exit on "already-set" and leave the
    // dialog stuck on "No compatible agent profiles".
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("incompat-id"));
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];
    const all = [makeProfile("incompat-id"), makeProfile("compat-id")];
    const compatible = [makeProfile("compat-id")];

    renderHook(() => useWorkflowAgentProfileEffect(fs, workflows, all, compatible));

    expect(fs.setAgentProfileId).not.toHaveBeenCalledWith("incompat-id");
    // Empty hand-off lets useDefaultSelectionsEffect pick a compatible default.
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("");
  });

  it("clears agentProfileId when the workflow has no override and there is no last-used id", () => {
    const fs = makeFs({ selectedWorkflowId: "wf-1" });
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile("real-id")],
        [makeProfile("real-id")],
      ),
    );

    // Empty string lets useDefaultSelectionsEffect take over the fallback
    // chain (workspace default → first profile).
    expect(fs.setAgentProfileId).toHaveBeenCalledWith("");
  });
});

describe("useWorkflowAgentProfileEffect — settings fallback readiness", () => {
  it("clears stale workflow agents before deferring last-used restore", async () => {
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];
    const fsBefore = makeFs({ selectedWorkflowId: "wf-1", executorProfileId: "" });

    const { rerender } = renderHook(
      ({ fs, compatible }) =>
        useWorkflowAgentProfileEffect(fs, workflows, [claude, cursor], compatible, {
          lastUsedAgentProfileId: claude.id,
        }),
      { initialProps: { fs: fsBefore, compatible: [claude, cursor] } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fsBefore.setAgentProfileId).toHaveBeenCalledWith("");
    expect(fsBefore.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);

    const fsAfter = makeFs({ selectedWorkflowId: "wf-1", executorProfileId: "profile-1" });
    rerender({ fs: fsAfter, compatible: [cursor] });

    await waitFor(() => expect(fsAfter.setAgentProfileId).toHaveBeenCalledWith(""));
    expect(fsAfter.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);
  });

  it("defers workflow last-used restore until auth compatibility is ready", async () => {
    const claude = makeProfile("claude");
    const workflows = [{ id: "wf-1" /* no agent_profile_id */ }];
    const fsBefore = makeFs({ selectedWorkflowId: "wf-1", executorProfileId: "profile-1" });

    const { rerender } = renderHook(
      ({ fs, authLoaded }) =>
        useWorkflowAgentProfileEffect(fs, workflows, [claude], [claude], {
          lastUsedAgentProfileId: claude.id,
          authLoaded,
        }),
      { initialProps: { fs: fsBefore, authLoaded: false } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fsBefore.setAgentProfileId).toHaveBeenCalledWith("");
    expect(fsBefore.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);

    const fsAfter = makeFs({ selectedWorkflowId: "wf-1", executorProfileId: "profile-1" });
    rerender({ fs: fsAfter, authLoaded: true });

    await waitFor(() => expect(fsAfter.setAgentProfileId).toHaveBeenCalledWith(claude.id));
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
  | "remoteRepos"
  | "useRemote"
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
    remoteRepos: [],
    useRemote: false,
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

describe("decideAgentProfileAutopick — user settings deferral", () => {
  it("defers agent auto-pick until user settings have loaded or settled", () => {
    const cursor = makeProfile("cursor");
    const deferred = decideAgentProfileAutopick({
      open: true,
      agentProfileId: "",
      workflowAgentProfileId: "",
      workflowHasAgent: false,
      agentProfiles: [cursor],
      compatibleAgentProfiles: [cursor],
      authLoaded: true,
      executorProfileId: "",
      hasExecutors: false,
      lastAgentProfileId: null,
      userSettingsLoaded: false,
      defaultAgentProfileId: null,
    });

    expect(deferred).toEqual({ kind: "defer", reason: "user-settings-not-loaded" });

    const picked = decideAgentProfileAutopick({
      open: true,
      agentProfileId: "",
      workflowAgentProfileId: "",
      workflowHasAgent: false,
      agentProfiles: [cursor],
      compatibleAgentProfiles: [cursor],
      authLoaded: true,
      executorProfileId: "",
      hasExecutors: false,
      lastAgentProfileId: null,
      userSettingsLoaded: true,
      defaultAgentProfileId: null,
    });

    expect(picked).toEqual({ kind: "pick", source: "first", id: cursor.id });
  });

  it("defers auto-pick while user settings load even when localStorage has a valid profile", () => {
    const cursor = makeProfile("cursor");
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify(cursor.id));

    const deferred = decideAgentProfileAutopick({
      open: true,
      agentProfileId: "",
      workflowAgentProfileId: "",
      workflowHasAgent: false,
      agentProfiles: [cursor],
      compatibleAgentProfiles: [cursor],
      authLoaded: true,
      executorProfileId: "",
      hasExecutors: false,
      lastAgentProfileId: cursor.id,
      userSettingsLoaded: false,
      defaultAgentProfileId: null,
    });

    expect(deferred).toEqual({ kind: "defer", reason: "user-settings-not-loaded" });
  });
});

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
