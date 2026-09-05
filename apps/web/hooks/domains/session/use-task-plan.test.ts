import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskPlan } from "@/lib/types/http";

const planApiMock = vi.hoisted(() => ({
  getTaskPlan: vi.fn(),
  createTaskPlan: vi.fn(),
  updateTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  listPlanRevisions: vi.fn(),
  getPlanRevision: vi.fn(),
  revertPlanRevision: vi.fn(),
}));
vi.mock("@/lib/api/domains/plan-api", () => planApiMock);

type MockTaskPlansState = {
  byTaskId: Record<string, TaskPlan | null>;
  loadingByTaskId: Record<string, boolean>;
  loadedByTaskId: Record<string, boolean>;
  savingByTaskId: Record<string, boolean>;
  revisionsByTaskId: Record<string, unknown[]>;
  revisionsLoadingByTaskId: Record<string, boolean>;
  revisionsLoadedByTaskId: Record<string, boolean>;
  revisionContentCache: Record<string, string>;
  previewRevisionIdByTaskId: Record<string, string | null>;
  comparePairByTaskId: Record<string, [string | null, string | null]>;
};

type MockState = {
  taskPlans: MockTaskPlansState;
  connection: { status: string };
  setTaskPlan: ReturnType<typeof vi.fn>;
  setTaskPlanLoading: ReturnType<typeof vi.fn>;
  setTaskPlanSaving: ReturnType<typeof vi.fn>;
  markTaskPlanSeen: ReturnType<typeof vi.fn>;
  setPlanRevisions: ReturnType<typeof vi.fn>;
  setPlanRevisionsLoading: ReturnType<typeof vi.fn>;
  cachePlanRevisionContent: ReturnType<typeof vi.fn>;
  setPreviewRevision: ReturnType<typeof vi.fn>;
  toggleComparePair: ReturnType<typeof vi.fn>;
  clearComparePair: ReturnType<typeof vi.fn>;
};

let mockState: MockState;

function freshState(): MockState {
  return {
    taskPlans: {
      byTaskId: {},
      loadingByTaskId: {},
      // isLoaded=true for every task by default so the mount-time fetchPlan
      // effect (and its loadRevisions twin) does not fire and race the
      // explicit calls each test makes.
      loadedByTaskId: new Proxy({}, { get: () => true }) as Record<string, boolean>,
      savingByTaskId: {},
      revisionsByTaskId: {},
      revisionsLoadingByTaskId: {},
      revisionsLoadedByTaskId: new Proxy({}, { get: () => true }) as Record<string, boolean>,
      revisionContentCache: {},
      previewRevisionIdByTaskId: {},
      comparePairByTaskId: {},
    },
    connection: { status: "connected" },
    setTaskPlan: vi.fn(),
    setTaskPlanLoading: vi.fn(),
    setTaskPlanSaving: vi.fn(),
    markTaskPlanSeen: vi.fn(),
    setPlanRevisions: vi.fn(),
    setPlanRevisionsLoading: vi.fn(),
    cachePlanRevisionContent: vi.fn(),
    setPreviewRevision: vi.fn(),
    toggleComparePair: vi.fn(),
    clearComparePair: vi.fn(),
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

import { useTaskPlan } from "./use-task-plan";
import { WebSocketRequestError } from "@/lib/ws/client";

const TASK_A = "task-a";
const TASK_B = "task-b";
const READ_FAILED = "read failed";
const CONTENT_TOO_LARGE = "content-too-large";

function sizeRejection(limit: number, submitted: number) {
  return new WebSocketRequestError("plan content is too large", "VALIDATION_ERROR", {
    reason: "plan_content_too_large",
    limit,
    submitted,
  });
}

function resetPlanApiMocks() {
  mockState = freshState();
  planApiMock.getTaskPlan.mockReset();
  planApiMock.createTaskPlan.mockReset();
  planApiMock.updateTaskPlan.mockReset();
  planApiMock.deleteTaskPlan.mockReset();
  planApiMock.listPlanRevisions.mockReset();
  planApiMock.getPlanRevision.mockReset();
  planApiMock.revertPlanRevision.mockReset();
}

describe("useTaskPlan saveError classification and scoping", () => {
  beforeEach(resetPlanApiMocks);

  afterEach(() => {
    cleanup();
  });

  it("classifies a size rejection into kind content-too-large with limit/submitted from details", async () => {
    planApiMock.createTaskPlan.mockRejectedValue(sizeRejection(262144, 300000));
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("x".repeat(300000));
    });

    expect(result.current.saveError).toEqual({
      kind: CONTENT_TOO_LARGE,
      limit: 262144,
      submitted: 300000,
    });
    // The draft is not touched: the store's plan setter is never called on rejection.
    expect(mockState.setTaskPlan).not.toHaveBeenCalled();
  });

  it("classifies a rejection with no reason, or a different reason, as generic", async () => {
    planApiMock.createTaskPlan.mockRejectedValueOnce(
      new WebSocketRequestError("boom", "INTERNAL_ERROR"),
    );
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("content");
    });
    expect(result.current.saveError).toEqual({ kind: "generic", message: "boom" });

    planApiMock.createTaskPlan.mockRejectedValueOnce(
      new WebSocketRequestError("other failure", "VALIDATION_ERROR", { reason: "something_else" }),
    );
    await act(async () => {
      await result.current.savePlan("content");
    });
    expect(result.current.saveError).toEqual({ kind: "generic", message: "other failure" });
  });

  it("a plain transport failure with no backend details also classifies as generic", async () => {
    planApiMock.createTaskPlan.mockRejectedValue(new Error("WebSocket client not available"));
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("content");
    });

    expect(result.current.saveError).toEqual({
      kind: "generic",
      message: "WebSocket client not available",
    });
  });

  it("clears the displayed rejection when the next save attempt begins, and admits a fixed draft", async () => {
    planApiMock.createTaskPlan.mockRejectedValueOnce(sizeRejection(262144, 300000));
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("oversized");
    });
    expect(result.current.saveError?.kind).toBe(CONTENT_TOO_LARGE);

    const savedPlan = { task_id: TASK_A, content: "short", title: "Plan" } as TaskPlan;
    planApiMock.createTaskPlan.mockResolvedValueOnce(savedPlan);
    await act(async () => {
      await result.current.savePlan("short");
    });
    expect(result.current.saveError).toBeNull();
  });

  it("reports the rejection's own limit/submitted, never re-derived from the submitted content's length (AC-003.5)", async () => {
    // The rejection's numbers are deliberately far from the submitted
    // string's own length, so a classifier that (incorrectly) measured the
    // draft instead of reading `details` would be caught immediately.
    planApiMock.createTaskPlan.mockRejectedValue(sizeRejection(262144, 999999));
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("short content, nowhere near 999999 bytes");
    });

    expect(result.current.saveError).toEqual({
      kind: CONTENT_TOO_LARGE,
      limit: 262144,
      submitted: 999999,
    });
  });

  it("reports generic for a non-size failure even when the submitted content is itself oversized (AC-003.5)", async () => {
    planApiMock.createTaskPlan.mockRejectedValue(
      new WebSocketRequestError("transient failure", "INTERNAL_ERROR"),
    );
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("x".repeat(300000));
    });

    expect(result.current.saveError).toEqual({ kind: "generic", message: "transient failure" });
  });
});

describe("useTaskPlan saveError clears at attempt begin, not completion (AC-003.5)", () => {
  beforeEach(resetPlanApiMocks);

  afterEach(() => {
    cleanup();
  });

  it("clears the displayed rejection when the next attempt begins, not when it completes", async () => {
    // Regression-shaped test: clearing saveError only in savePlan's success
    // branch (rather than at the start of every attempt) would leave a stale
    // rejection on screen for the attempt's entire in-flight duration. This
    // uses a controllable promise to observe state BEFORE the second attempt
    // resolves, which an always-awaited test cannot distinguish.
    planApiMock.createTaskPlan.mockRejectedValueOnce(sizeRejection(262144, 300000));
    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.savePlan("oversized");
    });
    expect(result.current.saveError?.kind).toBe(CONTENT_TOO_LARGE);

    let resolveSecondAttempt: (value: unknown) => void = () => {};
    const secondAttempt = new Promise((resolve) => {
      resolveSecondAttempt = resolve;
    });
    planApiMock.createTaskPlan.mockReturnValueOnce(secondAttempt);

    let secondSavePromise: Promise<unknown> = Promise.resolve(null);
    act(() => {
      secondSavePromise = result.current.savePlan("short");
    });

    // The second attempt has begun but not yet resolved — the prior
    // rejection must already be gone.
    expect(result.current.saveError).toBeNull();

    await act(async () => {
      resolveSecondAttempt({ task_id: TASK_A, content: "short", title: "Plan" });
      await secondSavePromise;
    });
    expect(result.current.saveError).toBeNull();
  });
});

describe("useTaskPlan saveError scoping to write attempts (AC-003.7)", () => {
  beforeEach(resetPlanApiMocks);

  afterEach(() => {
    cleanup();
  });

  it("does not report a rejection for fetchPlan, loadRevisions, revertTo, or deletePlan failures (AC-003.7)", async () => {
    planApiMock.getTaskPlan.mockRejectedValue(new Error(READ_FAILED));
    planApiMock.listPlanRevisions.mockRejectedValue(new Error(READ_FAILED));
    planApiMock.revertPlanRevision.mockRejectedValue(new Error(READ_FAILED));
    planApiMock.deleteTaskPlan.mockRejectedValue(new Error(READ_FAILED));

    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.refetch();
    });
    expect(result.current.saveError).toBeNull();

    await act(async () => {
      await result.current.loadRevisions();
    });
    expect(result.current.saveError).toBeNull();

    await act(async () => {
      await result.current.revertTo("rev-1");
    });
    expect(result.current.saveError).toBeNull();

    await act(async () => {
      await result.current.deletePlan();
    });
    expect(result.current.saveError).toBeNull();
  });

  it("does not report a rejection when opening a task whose stored plan already exceeds the ceiling (AC-003.7, AC-001.10)", async () => {
    const oversizedStoredPlan = {
      task_id: TASK_A,
      content: "x".repeat(300000),
      title: "Plan",
    } as TaskPlan;
    planApiMock.getTaskPlan.mockResolvedValue(oversizedStoredPlan);

    const { result } = renderHook(() => useTaskPlan(TASK_A));

    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.saveError).toBeNull();
    expect(mockState.setTaskPlan).toHaveBeenCalledWith(TASK_A, oversizedStoredPlan);
  });
});

describe("useTaskPlan saveError attempt ordering (AC-003.8)", () => {
  beforeEach(resetPlanApiMocks);

  afterEach(() => {
    cleanup();
  });

  it("resets saveError when the task changes (AC-003.8)", async () => {
    planApiMock.createTaskPlan.mockRejectedValue(sizeRejection(262144, 300000));
    const { result, rerender } = renderHook(({ taskId }) => useTaskPlan(taskId), {
      initialProps: { taskId: TASK_A as string | null },
    });

    await act(async () => {
      await result.current.savePlan("oversized");
    });
    expect(result.current.saveError?.kind).toBe(CONTENT_TOO_LARGE);

    rerender({ taskId: TASK_B });
    await waitFor(() => expect(result.current.saveError).toBeNull());
  });

  it("drops a stale-task rejection: a save for task A that fails after switching to task B is not displayed", async () => {
    let rejectA!: (err: unknown) => void;
    planApiMock.createTaskPlan.mockReturnValue(
      new Promise((_resolve, reject) => {
        rejectA = reject;
      }),
    );

    const { result, rerender } = renderHook(({ taskId }) => useTaskPlan(taskId), {
      initialProps: { taskId: TASK_A as string | null },
    });

    let saveAPromise!: Promise<TaskPlan | null>;
    act(() => {
      saveAPromise = result.current.savePlan("oversized");
    });

    // Switch to task B before A's rejection arrives.
    rerender({ taskId: TASK_B });

    await act(async () => {
      rejectA(sizeRejection(262144, 300000));
      await saveAPromise;
    });

    expect(result.current.saveError).toBeNull();
  });

  it("drops a stale rejection across an A->B->A round trip, even though no newer A attempt exists (AC-003.8)", async () => {
    // Regression test: isLatestForTask alone survives a round trip back to
    // the same task unchanged, because nothing re-attempted a save on A in
    // between — so the guard used to treat the original A attempt as still
    // current once the panel returned to A, displaying a rejection for a
    // write the user never saw fail while away from A. AC-003.8: "a write
    // for the previous task that fails after the change shall not be
    // displayed at all" - not just "not displayed while away from it".
    let rejectA!: (err: unknown) => void;
    planApiMock.createTaskPlan.mockReturnValue(
      new Promise((_resolve, reject) => {
        rejectA = reject;
      }),
    );

    const { result, rerender } = renderHook(({ taskId }) => useTaskPlan(taskId), {
      initialProps: { taskId: TASK_A as string | null },
    });

    let saveAPromise!: Promise<TaskPlan | null>;
    act(() => {
      saveAPromise = result.current.savePlan("oversized");
    });

    // A -> B -> A before A's rejection arrives.
    rerender({ taskId: TASK_B });
    rerender({ taskId: TASK_A });

    await act(async () => {
      rejectA(sizeRejection(262144, 300000));
      await saveAPromise;
    });

    expect(result.current.saveError).toBeNull();
  });

  it("keeps the outcome of the later-started attempt when the earlier one resolves last", async () => {
    let resolveFirst!: (err: unknown) => void;
    let resolveSecond!: (plan: TaskPlan) => void;
    planApiMock.createTaskPlan
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          resolveFirst = reject;
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveSecond = resolve;
        }),
      );

    const { result } = renderHook(() => useTaskPlan(TASK_A));

    let firstPromise!: Promise<TaskPlan | null>;
    let secondPromise!: Promise<TaskPlan | null>;
    act(() => {
      firstPromise = result.current.savePlan("first attempt");
      secondPromise = result.current.savePlan("second attempt");
    });

    // The later-started attempt (second) resolves first, then the earlier
    // (first) resolves after it — its rejection must not overwrite the
    // second attempt's outcome.
    await act(async () => {
      resolveSecond({ task_id: TASK_A, content: "second attempt", title: "Plan" } as TaskPlan);
      await secondPromise;
    });
    expect(result.current.saveError).toBeNull();

    await act(async () => {
      resolveFirst(sizeRejection(262144, 300000));
      await firstPromise;
    });
    expect(result.current.saveError).toBeNull();
  });
});

describe("useTaskPlan plan-store write scoping (AC-003.2)", () => {
  beforeEach(resetPlanApiMocks);

  afterEach(() => {
    cleanup();
  });

  it("does not let an earlier-started successful save overwrite the store after a later-started save was rejected", async () => {
    // Regression test: savePlan's success branch called setTaskPlan
    // unconditionally, with no guard against a stale, earlier-started
    // attempt resolving after a later-started attempt was already rejected.
    // The panel's "sync from external plan updates" effect then treats that
    // stale write as a real external change and overwrites the user's
    // rejected draft with it — exactly what AC-003.2 forbids.
    let resolveFirst!: (plan: TaskPlan) => void;
    let rejectSecond!: (err: unknown) => void;
    planApiMock.createTaskPlan
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectSecond = reject;
        }),
      );

    const { result } = renderHook(() => useTaskPlan(TASK_A));

    let firstPromise!: Promise<TaskPlan | null>;
    let secondPromise!: Promise<TaskPlan | null>;
    act(() => {
      firstPromise = result.current.savePlan("first attempt");
      secondPromise = result.current.savePlan("second attempt, oversized");
    });

    // The later-started attempt (second) is rejected first.
    await act(async () => {
      rejectSecond(sizeRejection(262144, 300000));
      await secondPromise;
    });
    expect(result.current.saveError?.kind).toBe(CONTENT_TOO_LARGE);
    expect(mockState.setTaskPlan).not.toHaveBeenCalled();

    // The earlier-started attempt (first) then succeeds. Its stale success
    // must not reach the store: doing so would feed the panel's external-sync
    // effect and clobber the still-rejected draft with content the user never
    // saw accepted.
    await act(async () => {
      resolveFirst({ task_id: TASK_A, content: "first attempt", title: "Plan" } as TaskPlan);
      await firstPromise;
    });
    expect(mockState.setTaskPlan).not.toHaveBeenCalled();
    expect(result.current.saveError?.kind).toBe(CONTENT_TOO_LARGE);
  });

  it("still writes a legitimate background success to the store after switching tasks, since no newer attempt exists for that task", async () => {
    let resolveA!: (plan: TaskPlan) => void;
    planApiMock.createTaskPlan.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveA = resolve;
      }),
    );

    const { result, rerender } = renderHook(({ taskId }) => useTaskPlan(taskId), {
      initialProps: { taskId: TASK_A as string | null },
    });

    let saveAPromise!: Promise<TaskPlan | null>;
    act(() => {
      saveAPromise = result.current.savePlan("content");
    });

    // Switching tasks must not, by itself, make a still-in-flight save for
    // the PREVIOUS task look stale — only a newer attempt for that SAME task
    // should.
    rerender({ taskId: TASK_B });

    const savedPlan = { task_id: TASK_A, content: "content", title: "Plan" } as TaskPlan;
    await act(async () => {
      resolveA(savedPlan);
      await saveAPromise;
    });

    expect(mockState.setTaskPlan).toHaveBeenCalledWith(TASK_A, savedPlan);
  });
});
