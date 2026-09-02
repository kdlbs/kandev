import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlanSaveError } from "@/hooks/domains/session/use-task-plan";
import { usePlanDraft } from "@/hooks/domains/session/use-plan-draft";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: null }, taskSessions: { items: {} } }),
  useAppStoreApi: () => ({ getState: () => ({}) }),
}));

const AUTO_SAVE_DELAY = 1500;
const REJECTED_CONTENT = "rejected content";
const TASK_1 = "task-1";
const TASK_2 = "task-2";
// Autosave suppression only applies to a size rejection (AC-003.4's "such a
// rejection" chains back to AC-003.1's ceiling rejection); every suppression
// test in this file simulates one via this classified saveError.
const SIZE_REJECTION: PlanSaveError = {
  kind: "content-too-large",
  limit: 262144,
  submitted: 300000,
};
const GENERIC_REJECTION: PlanSaveError = { kind: "generic", message: "temporary failure" };

type DraftPlan = { content?: string; title?: string } | null;
type DraftProps = {
  plan: DraftPlan;
  isSaving: boolean;
  taskId: string | null;
  saveError?: PlanSaveError | null;
};

function renderDraft(
  plan: DraftPlan,
  savePlan: (content: string, title?: string) => Promise<unknown>,
  taskId: string | null = TASK_1,
  saveError: PlanSaveError | null = SIZE_REJECTION,
) {
  const editorWrapperRef = { current: null };
  return renderHook(
    ({ plan: p, isSaving, taskId: id, saveError = null }: DraftProps) =>
      usePlanDraft({
        plan: p,
        isSaving,
        savePlan,
        editorWrapperRef,
        taskId: id,
        saveError,
      }),
    { initialProps: { plan, isSaving: false, taskId, saveError } as DraftProps },
  );
}

/**
 * Simulates the isSaving true -> false round trip a real savePlan call
 * produces via the store (setTaskPlanSaving). The autosave effect depends on
 * isSaving, so it only gets a chance to re-run — and the suppression guard a
 * chance to matter — across exactly this transition. A test that never
 * drives it can pass whether or not the guard is even present.
 */
async function settleSaveRoundTrip(
  rerender: (props: DraftProps) => void,
  plan: DraftPlan,
  taskId: string | null,
  saveError: PlanSaveError | null = null,
) {
  rerender({ plan, isSaving: true, taskId, saveError: null });
  await act(async () => {
    await Promise.resolve();
  });
  rerender({ plan, isSaving: false, taskId, saveError });
}

describe("usePlanDraft autosave suppression (AC-003.4)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("does not resubmit rejected content across an isSaving round trip", async () => {
    const savePlan = vi.fn().mockResolvedValue(null); // simulates a rejected save (savePlan resolves null on failure)
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent("oversized content");
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });

    // First debounce fires the initial (rejected) attempt.
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    // The isSaving round trip a rejected save produces in the real app —
    // this is the transition the autosave effect depends on, and a version
    // of this hook without the suppression ref re-arms across it and
    // resubmits indefinitely.
    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);

    // Advance well past several more debounce intervals.
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY * 5);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);
  });

  it("re-arms once the draft changes away from the rejected content", async () => {
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);

    act(() => {
      result.current.setDraftContent("a different, shorter draft");
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });

  it("an explicit save (attemptSave) resubmits even unchanged content", async () => {
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);

    // Simulate the Ctrl/Cmd+S escape hatch: useSaveShortcut calls
    // attemptSave directly, bypassing the autosave debounce entirely.
    await act(async () => {
      await result.current.attemptSave(REJECTED_CONTENT);
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });
});

describe("usePlanDraft autosave suppression is scoped to size rejections", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("re-arms after a generic (non-size) failure once its isSaving round trip settles", async () => {
    // Regression test: the suppression guard used to key only on "was the
    // just-attempted content rejected", not on why — so a transient
    // transport/server failure (classified "generic", not
    // "content-too-large") permanently blocked autosave for an unchanged
    // draft, with no self-healing once the backend recovered. AC-003.4's
    // "such a rejection" is scoped to AC-003.1's ceiling rejection, so only
    // a size rejection may suppress; a generic failure must keep retrying.
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1 });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, {
      kind: "generic",
      message: "network error",
    });

    // No draft change and no explicit save — a size rejection would stay
    // suppressed here, but a generic failure must re-arm on its own.
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });
});

describe("usePlanDraft explicit save (Ctrl/Cmd+S) interactions (AC-003.3/003.4)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("still reports unsaved changes after a rejected save (AC-003.3)", async () => {
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    expect(result.current.hasUnsavedChanges).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);

    // Never presented as saved: the plan is untouched (savePlan resolved
    // null, so the store's plan was never updated) and the draft still
    // differs from it.
    expect(result.current.hasUnsavedChanges).toBe(true);
  });

  it("does not automatically resubmit after an explicit save is itself rejected", async () => {
    // Regression test: useSaveShortcut used to clear the suppression ref
    // without recording what it had just submitted, so once its own
    // rejected attempt's isSaving round trip settled, the autosave effect
    // saw no suppression match and re-armed on its own — resubmitting the
    // same still-oversized draft with no further user action.
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);

    // Explicit save (Ctrl/Cmd+S) on unchanged, still-rejected content.
    await act(async () => {
      await result.current.attemptSave(REJECTED_CONTENT);
    });
    expect(savePlan).toHaveBeenCalledTimes(2);

    // The explicit save's own isSaving round trip settles, with no further
    // draft change or user action afterward.
    await settleSaveRoundTrip(rerender, plan, TASK_1, SIZE_REJECTION);
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY * 3);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });
});

describe("usePlanDraft autosave suppression reset (AC-003.4)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("clears the suppression on a successful save so a later edit can autosave again", async () => {
    const savePlan = vi.fn().mockResolvedValue({ content: "content that will save" });
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    act(() => {
      result.current.setDraftContent("content that will save");
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    // The store's plan now reflects the saved content, matching the current
    // draft exactly — same as the real flow, where a successful save's
    // result flows back through the store before the next edit.
    const updatedPlan = { content: "content that will save" };
    rerender({ plan: updatedPlan, isSaving: false, taskId: TASK_1, saveError: null });

    // A further, real edit re-diverges the draft from the plan.
    act(() => {
      result.current.setDraftContent("content that will save again");
    });
    rerender({ plan: updatedPlan, isSaving: false, taskId: TASK_1, saveError: null });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });

  it("resets the draft and suppression when switching between tasks without plans", async () => {
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = null;
    const { result, rerender } = renderDraft(plan, savePlan, TASK_1);

    act(() => {
      result.current.setDraftContent(REJECTED_CONTENT);
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    // Switch to a task with no plan. The previous task's rejected draft must
    // not remain visible or get sent to the new task.
    rerender({ plan: null, isSaving: false, taskId: TASK_2, saveError: null });
    expect(result.current.draftContent).toBe("");
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY * 3);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    // A new edit on task B is the only event that may arm its autosave.
    act(() => {
      result.current.setDraftContent("task B content");
    });
    rerender({ plan: null, isSaving: false, taskId: TASK_2, saveError: null });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
    expect(savePlan).toHaveBeenLastCalledWith("task B content", undefined);
  });

  it("retries unchanged content after a generic failure", async () => {
    const savePlan = vi.fn().mockResolvedValue(null);
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan, TASK_1, SIZE_REJECTION);

    act(() => {
      result.current.setDraftContent("temporarily unavailable");
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(1);

    await settleSaveRoundTrip(rerender, plan, TASK_1, GENERIC_REJECTION);
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });
});

describe("usePlanDraft overlapping attemptSave calls (AC-003.4 regression)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("does not let an earlier-started successful save clear suppression for a later-started rejected save", async () => {
    // Regression test: attemptSave cleared the shared suppression ref
    // whenever ANY attempt succeeded, without checking the ref still held
    // that attempt's own content. Two overlapping attempts (an explicit save
    // racing an in-flight autosave) can resolve out of start order: if the
    // earlier-started one succeeds after the later-started one was already
    // rejected, the earlier one's success must not erase the suppression the
    // later, rejected attempt put in place.
    let resolveA: (value: unknown) => void = () => {};
    let resolveB: (value: unknown) => void = () => {};
    const promiseA = new Promise((resolve) => {
      resolveA = resolve;
    });
    const promiseB = new Promise((resolve) => {
      resolveB = resolve;
    });
    const savePlan = vi.fn((content: string) => (content === "content-A" ? promiseA : promiseB));
    const plan = { content: "" };
    const { result, rerender } = renderDraft(plan, savePlan);

    let attemptAResult: Promise<unknown> = Promise.resolve(null);
    let attemptBResult: Promise<unknown> = Promise.resolve(null);
    act(() => {
      attemptAResult = result.current.attemptSave("content-A");
    });
    act(() => {
      attemptBResult = result.current.attemptSave("content-B");
    });
    expect(savePlan).toHaveBeenCalledTimes(2);

    // The later-started attempt (B) resolves first, rejected.
    await act(async () => {
      resolveB(null);
      await attemptBResult;
    });

    // The earlier-started attempt (A) resolves after, successfully.
    await act(async () => {
      resolveA({ content: "content-A" });
      await attemptAResult;
    });

    // The draft still holds B's rejected content, unedited, with no explicit
    // save. Autosave must stay suppressed.
    act(() => {
      result.current.setDraftContent("content-B");
    });
    rerender({ plan, isSaving: false, taskId: TASK_1, saveError: SIZE_REJECTION });
    await act(async () => {
      vi.advanceTimersByTime(AUTO_SAVE_DELAY * 3);
      await Promise.resolve();
    });
    expect(savePlan).toHaveBeenCalledTimes(2);
  });
});
