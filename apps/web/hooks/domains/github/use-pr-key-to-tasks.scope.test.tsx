import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, renderHook } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { PRRowTaskIndicator } from "@/components/github/my-github/pr-row-task-indicator";
import type { HydrationState } from "@/lib/state/store";
import type { TaskPR, TaskPRsResponse } from "@/lib/types/github";
import { prKey, usePRKeyToTasks } from "./use-pr-key-to-tasks";

const transport = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/github-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/github-api")>()),
  listWorkspaceTaskPRs: transport.list,
}));

function deferredResponse() {
  let resolve!: (value: TaskPRsResponse) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<TaskPRsResponse>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const TASK_A_TITLE = "Task from workspace A";
const EMPTY_INDICATOR = "pr-row-task-indicator-empty";
const PR_KEY = "owner/repo#7";

const requests: ReturnType<typeof deferredResponse>[] = [];

beforeEach(() => {
  transport.list.mockReset();
  transport.list.mockImplementation(() => {
    const request = deferredResponse();
    requests.push(request);
    return request.promise;
  });
});

afterEach(async () => {
  cleanup();
  requests.splice(0).forEach((request) => request.resolve({ task_prs: {} }));
  await Promise.resolve();
  await Promise.resolve();
});

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "association-a",
    workspace_id: WORKSPACE_A,
    task_id: "task-a",
    owner: "owner",
    repo: "repo",
    pr_number: 7,
    pr_url: "https://github.com/owner/repo/pull/7",
    pr_title: TASK_A_TITLE,
    head_branch: "feature",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

type ScopeFixture = {
  activeId?: string | null;
  generation?: number;
  cacheWorkspaceId?: string;
  cacheGeneration?: number;
  byTaskId?: Record<string, TaskPR[]>;
};

function scopeState({
  activeId = WORKSPACE_A,
  generation = 1,
  cacheWorkspaceId = WORKSPACE_A,
  cacheGeneration = 1,
  byTaskId = { "task-a": [makePR()] },
}: ScopeFixture = {}): HydrationState {
  return {
    workspaces: { items: [], activeId },
    workspaceContextGeneration: generation,
    taskPRs: {
      workspaceId: cacheWorkspaceId,
      workspaceContextGeneration: cacheGeneration,
      byTaskId,
    },
  };
}

function providerFor(initialState: HydrationState) {
  return function ScopeProvider({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={initialState}>
        <TooltipProvider>{children}</TooltipProvider>
      </StateProvider>
    );
  };
}

function renderAssociations(initialState = scopeState(), workspaceId: string | null = WORKSPACE_A) {
  return renderHook(
    ({ workspaceId }: { workspaceId: string | null }) => ({
      map: usePRKeyToTasks(workspaceId),
      store: useAppStoreApi(),
    }),
    { wrapper: providerFor(initialState), initialProps: { workspaceId } },
  );
}

function LinkedPR({ workspaceId }: { workspaceId: string | null }) {
  const map = usePRKeyToTasks(workspaceId);
  return <PRRowTaskIndicator tasks={map.get(prKey("owner", "repo", 7))} />;
}

function renderRow(initialState = scopeState(), workspaceId: string | null = WORKSPACE_A) {
  return render(<LinkedPR workspaceId={workspaceId} />, { wrapper: providerFor(initialState) });
}

async function publishResponse(index: number, taskPRs: Record<string, TaskPR[]>) {
  const request = requests[index];
  if (!request) throw new Error(`No workspace list request at index ${index}`);
  await act(async () => {
    request.resolve({ task_prs: taskPRs });
    await request.promise;
  });
}

describe("GitHub reverse association workspace scope", () => {
  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2, .5
  it("hides A links while requested B is pending", () => {
    const { result } = renderAssociations(
      scopeState({ activeId: WORKSPACE_B, generation: 2 }),
      WORKSPACE_B,
    );
    expect(transport.list).toHaveBeenCalledWith(WORKSPACE_B, { cache: "no-store" });
    expect(result.current.store.getState().taskPRs.byTaskId["task-a"]).toHaveLength(1);
    expect(result.current.map.size).toBe(0);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3
  it("rejects old generations", () => {
    const { result } = renderAssociations(scopeState({ generation: 2 }));
    expect(result.current.store.getState().taskPRs.workspaceContextGeneration).toBe(1);
    expect(result.current.map.size).toBe(0);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2
  it("hides links without a requested workspace", () => {
    const { result } = renderAssociations(scopeState(), null);
    expect(transport.list).not.toHaveBeenCalled();
    expect(result.current.map.size).toBe(0);
    const row = renderRow(scopeState(), null);
    expect(row.getByTestId(EMPTY_INDICATOR)).toBeTruthy();
    expect(row.queryByRole("button")).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2, .7
  it("hides the old task button on requested B row", () => {
    const row = renderRow(scopeState({ activeId: WORKSPACE_B, generation: 2 }), WORKSPACE_B);
    expect(row.queryByTestId("pr-row-task-indicator-single")).toBeNull();
    expect(row.queryByTestId("pr-row-task-indicator-multi")).toBeNull();
    expect(row.queryByRole("button")).toBeNull();
    expect(row.getByTestId(EMPTY_INDICATOR)).toBeTruthy();
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.1, .5, .7
  it("preserves current cached links during pending or failed reads", async () => {
    const pr = makePR();
    const initialState = scopeState({ byTaskId: { "task-a": [pr] } });
    const { result } = renderAssociations(initialState);
    const row = renderRow(initialState);
    expect(transport.list).toHaveBeenCalledWith(WORKSPACE_A, { cache: "no-store" });
    expect(requests).toHaveLength(2);
    expect(result.current.map.get(PR_KEY)?.[0]).toBe(pr);
    expect(row.getByRole("button", { name: TASK_A_TITLE })).toBeTruthy();
    expect(row.queryByTestId(EMPTY_INDICATOR)).toBeNull();
    await act(async () => {
      requests.forEach((request) => request.reject(new Error("workspace list unavailable")));
      await Promise.resolve();
    });
    expect(result.current.map.get(PR_KEY)?.[0]).toBe(pr);
    expect(row.getByRole("button", { name: TASK_A_TITLE })).toBeTruthy();
  });
});

describe("GitHub reverse association reactive scope", () => {
  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2, .4
  it("rejects a requested workspace different from active", () => {
    const pr = makePR();
    const { result, rerender } = renderAssociations(scopeState({ byTaskId: { "task-a": [pr] } }));
    const cache = result.current.store.getState().taskPRs.byTaskId;
    expect(result.current.map.get(PR_KEY)?.[0]).toBe(pr);
    rerender({ workspaceId: WORKSPACE_B });
    expect(result.current.store.getState().taskPRs.byTaskId).toBe(cache);
    expect(result.current.map.size).toBe(0);
    rerender({ workspaceId: WORKSPACE_A });
    expect(result.current.map.get(PR_KEY)?.[0]).toBe(pr);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3, .4
  it.each(["workspaceId", "workspaceContextGeneration"] as const)(
    "rejects a missing cache %s after initializer normalization",
    (stamp) => {
      const { result } = renderAssociations();
      const store = result.current.store;
      const cache = store.getState().taskPRs;
      expect(cache.workspaceId).toBe(WORKSPACE_A);
      expect(cache.workspaceContextGeneration).toBe(1);
      expect(result.current.map.size).toBe(1);
      act(() => {
        const withoutStamp = { ...cache };
        delete withoutStamp[stamp];
        store.setState({ taskPRs: withoutStamp });
      });
      expect(store.getState().taskPRs.byTaskId).toBe(cache.byTaskId);
      expect(store.getState().taskPRs[stamp]).toBeUndefined();
      expect(result.current.map.size).toBe(0);
    },
  );

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3, .4
  it("reacts to A B A and same-workspace generation changes", () => {
    const { result, rerender } = renderAssociations();
    const store = result.current.store;
    const cache = store.getState().taskPRs.byTaskId;
    expect(result.current.map.size).toBe(1);
    act(() => store.getState().setActiveWorkspace(WORKSPACE_B));
    expect(result.current.map.size).toBe(0);
    rerender({ workspaceId: WORKSPACE_B });
    expect(result.current.map.size).toBe(0);
    act(() => store.getState().setActiveWorkspace(WORKSPACE_A));
    rerender({ workspaceId: WORKSPACE_A });
    expect(store.getState().workspaceContextGeneration).toBe(3);
    expect(store.getState().taskPRs.byTaskId).toBe(cache);
    expect(result.current.map.size).toBe(0);
    act(() =>
      store.getState().setTaskPRs(cache, {
        workspaceId: WORKSPACE_A,
        workspaceContextGeneration: store.getState().workspaceContextGeneration,
      }),
    );
    expect(result.current.map.size).toBe(1);
    act(() => store.getState().resetKanbanWorkspaceContext());
    expect(store.getState().taskPRs.byTaskId).toBe(cache);
    expect(result.current.map.size).toBe(0);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.1, .4
  it("reacts when only cache scope metadata changes", () => {
    const { result } = renderAssociations();
    const store = result.current.store;
    const cache = store.getState().taskPRs;
    expect(result.current.map.size).toBe(1);
    act(() => store.setState({ taskPRs: { ...cache, workspaceContextGeneration: 0 } }));
    expect(store.getState().taskPRs.byTaskId).toBe(cache.byTaskId);
    expect(result.current.map.size).toBe(0);
    act(() => store.setState({ taskPRs: cache }));
    expect(result.current.map.size).toBe(1);
    act(() => store.setState({ taskPRs: { ...cache, workspaceId: WORKSPACE_B } }));
    expect(store.getState().taskPRs.byTaskId).toBe(cache.byTaskId);
    expect(result.current.map.size).toBe(0);
  });
});

describe("GitHub reverse association response scope", () => {
  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.5
  it("keeps stale links empty after failure", async () => {
    const initialState = scopeState({ activeId: WORKSPACE_B, generation: 2 });
    const { result } = renderAssociations(initialState, WORKSPACE_B);
    const row = renderRow(initialState, WORKSPACE_B);
    await act(async () => {
      requests.forEach((request) => request.reject(new Error("workspace list unavailable")));
      await Promise.resolve();
    });
    expect(result.current.map.size).toBe(0);
    expect(row.getByTestId(EMPTY_INDICATOR)).toBeTruthy();
    expect(row.queryByRole("button")).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.4, .5, .7
  it("accepts a current response and renders its task button", async () => {
    const initialState = scopeState({ activeId: WORKSPACE_B, generation: 2 });
    const { result } = renderAssociations(initialState, WORKSPACE_B);
    const row = renderRow(initialState, WORKSPACE_B);
    const pr = makePR({
      id: "association-b",
      workspace_id: WORKSPACE_B,
      task_id: "task-b",
      pr_title: "Task from workspace B",
    });
    await publishResponse(0, { "task-b": [pr] });
    await publishResponse(1, { "task-b": [pr] });
    expect(result.current.map.get(PR_KEY)).toHaveLength(1);
    expect(result.current.map.get(PR_KEY)?.[0]).toBe(pr);
    expect(row.getByRole("button", { name: "Task from workspace B" })).toBeTruthy();
    expect(row.queryByTestId(EMPTY_INDICATOR)).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3, .5
  it("keeps an obsolete response unreadable after a context change", async () => {
    const { result } = renderAssociations();
    const store = result.current.store;
    act(() => store.getState().resetKanbanWorkspaceContext());
    await publishResponse(0, { "task-a": [makePR()] });
    expect(store.getState().workspaceContextGeneration).toBe(2);
    expect(store.getState().taskPRs.workspaceContextGeneration).toBe(1);
    expect(store.getState().taskPRs.byTaskId["task-a"]).toHaveLength(1);
    expect(result.current.map.size).toBe(0);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.5, .7
  it("clears the reverse read on a current empty response", async () => {
    const { result } = renderAssociations();
    const row = renderRow();
    expect(result.current.map.size).toBe(1);
    expect(row.getByRole("button", { name: TASK_A_TITLE })).toBeTruthy();
    await publishResponse(0, {});
    await publishResponse(1, {});
    expect(result.current.map.size).toBe(0);
    expect(row.getByTestId(EMPTY_INDICATOR)).toBeTruthy();
    expect(row.queryByRole("button")).toBeNull();
  });
});

describe("GitHub reverse association identity and grouping", () => {
  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.6, .7
  it("isolates same-key links across independent stores", () => {
    const a = makePR();
    const b = makePR({ id: "association-b", workspace_id: WORKSPACE_B, task_id: "task-b" });
    const first = renderAssociations(scopeState({ byTaskId: { "task-a": [a] } }));
    const second = renderAssociations(
      scopeState({
        activeId: WORKSPACE_B,
        cacheWorkspaceId: WORKSPACE_B,
        byTaskId: { "task-b": [b] },
      }),
      WORKSPACE_B,
    );
    const obsolete = renderAssociations(scopeState({ activeId: WORKSPACE_B }), WORKSPACE_B);
    expect(first.result.current.store).not.toBe(second.result.current.store);
    expect(obsolete.result.current.store).not.toBe(second.result.current.store);
    expect(first.result.current.map.get(PR_KEY)?.[0]).toBe(a);
    expect(second.result.current.map.get(PR_KEY)).toHaveLength(1);
    expect(second.result.current.map.get(PR_KEY)?.[0]).toBe(b);
    expect(obsolete.result.current.map.size).toBe(0);
    act(() => obsolete.result.current.store.getState().resetKanbanWorkspaceContext());
    expect(first.result.current.map.get(PR_KEY)?.[0]).toBe(a);
    expect(second.result.current.map.get(PR_KEY)?.[0]).toBe(b);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.1, .6, .7
  it("preserves record identity and multi-task multi-repo grouping", () => {
    const a = makePR();
    const secondPR = makePR({ id: "association-a-next", pr_number: 8 });
    const secondRepo = makePR({ id: "association-a-other", repo: "other" });
    const b = makePR({ id: "association-b", task_id: "task-b", pr_title: "Another task" });
    const initialState = scopeState({
      byTaskId: { "task-a": [a, secondPR, secondRepo], "task-b": [b] },
    });
    const { result } = renderAssociations(initialState);
    const map = result.current.map;
    expect(map.size).toBe(3);
    expect(map.get(PR_KEY)).toHaveLength(2);
    expect(map.get(PR_KEY)?.[0]).toBe(a);
    expect(map.get(PR_KEY)?.[1]).toBe(b);
    expect(map.get("owner/repo#8")?.[0]).toBe(secondPR);
    expect(map.get("owner/other#7")?.[0]).toBe(secondRepo);
    const row = renderRow(initialState);
    expect(row.getByTestId("pr-row-task-indicator-multi").tagName).toBe("BUTTON");
    expect(row.getByRole("button", { name: "Tasks 2" })).toBeTruthy();
    expect(row.queryByTestId(EMPTY_INDICATOR)).toBeNull();
  });
});
