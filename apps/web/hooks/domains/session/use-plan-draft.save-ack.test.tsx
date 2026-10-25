import { useRef, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskPlan } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/client";
import { usePlanDraft } from "./use-plan-draft";
import { useTaskPlan } from "./use-task-plan";

const transport = vi.hoisted(() => ({
  create: vi.fn<(taskId: string, content: string, title?: string) => Promise<TaskPlan>>(),
  update: vi.fn<(taskId: string, content: string, title?: string) => Promise<TaskPlan>>(),
}));
vi.mock("@/lib/api/domains/plan-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/plan-api")>()),
  createTaskPlan: transport.create,
  updateTaskPlan: transport.update,
}));

const TASK_A = "draft-ack-task-a";
const TASK_B = "draft-ack-task-b";
const TITLE = "Plan";
const BASELINE = "Persisted outline";
const SUBMITTED = "Outline submitted for saving";
const NEWER = "Outline submitted for saving with newer notes";
const LATEST_SUBMITTED = "Later submitted outline";
const AUTO_SAVE_DELAY = 1500;

function planFor(taskId: string, content: string): TaskPlan {
  return {
    id: `plan-${taskId}`,
    task_id: taskId,
    title: TITLE,
    content,
    created_by: "user",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:01Z",
  };
}

function deferredSave() {
  let resolve!: (plan: TaskPlan) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<TaskPlan>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function useDraftHarness(taskId: string | null) {
  const store = useAppStoreApi();
  const saved = useTaskPlan(taskId);
  const editorWrapperRef = useRef<HTMLDivElement>(null);
  const draft = usePlanDraft({ ...saved, taskId, editorWrapperRef });
  return { store, saved, draft };
}

function mountDraft(initialPlan: TaskPlan | null = planFor(TASK_A, BASELINE)) {
  const view = renderHook(({ taskId }) => useDraftHarness(taskId), {
    initialProps: { taskId: TASK_A as string | null },
    wrapper,
  });
  act(() => view.result.current.store.getState().setTaskPlan(TASK_A, initialPlan));
  expect(view.result.current.draft.draftContent).toBe(initialPlan?.content ?? "");
  return view;
}

type DraftView = ReturnType<typeof mountDraft>;

function editDraft(view: DraftView, content: string) {
  act(() => view.result.current.draft.setDraftContent(content));
}

function submitExplicitly(view: DraftView, content: string) {
  let pending!: Promise<TaskPlan | null>;
  act(() => {
    view.result.current.draft.setDraftContent(content);
    pending = view.result.current.draft.attemptSave(content, TITLE);
  });
  return pending;
}

async function advanceAutosave(intervals = 1) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(AUTO_SAVE_DELAY * intervals);
  });
}

async function acknowledge(request: ReturnType<typeof deferredSave>, plan: TaskPlan) {
  await act(async () => {
    request.resolve(plan);
    await Promise.resolve();
  });
}

async function rejectSave(request: ReturnType<typeof deferredSave>, error: Error) {
  await act(async () => {
    request.reject(error);
    await Promise.resolve();
  });
}

function sizeRejection() {
  return new WebSocketRequestError("plan content too large", "VALIDATION_ERROR", {
    reason: "plan_content_too_large",
    limit: 262144,
    submitted: 300000,
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  transport.create
    .mockReset()
    .mockImplementation(async (taskId, content) => planFor(taskId, content));
  transport.update
    .mockReset()
    .mockImplementation(async (taskId, content) => planFor(taskId, content));
});

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("plan draft own save acknowledgements", () => {
  // @covers AC-TASKS-DOCUMENTS-003.1, AC-TASKS-DOCUMENTS-003.2
  it("keeps newer typing after its own autosave acknowledgement", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    editDraft(view, SUBMITTED);
    const editorKey = view.result.current.draft.editorKey;
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledExactlyOnceWith(TASK_A, SUBMITTED, TITLE);
    expect(view.result.current.saved.isSaving).toBe(true);

    editDraft(view, NEWER);
    await acknowledge(request, planFor(TASK_A, SUBMITTED));

    expect(view.result.current.store.getState().taskPlans.byTaskId[TASK_A]?.content).toBe(
      SUBMITTED,
    );
    expect(view.result.current.draft.draftContent).toBe(NEWER);
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
    expect(view.result.current.saved.isSaving).toBe(false);
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledTimes(2);
    expect(transport.update).toHaveBeenLastCalledWith(TASK_A, NEWER, TITLE);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
  });

  // @covers AC-TASKS-DOCUMENTS-003.1, AC-TASKS-DOCUMENTS-003.2
  it("keeps newer typing when the first plan is created and updates it next", async () => {
    const request = deferredSave();
    transport.create.mockImplementationOnce(() => request.promise);
    const view = mountDraft(null);
    editDraft(view, SUBMITTED);
    const editorKey = view.result.current.draft.editorKey;
    await advanceAutosave();
    expect(transport.create).toHaveBeenCalledExactlyOnceWith(TASK_A, SUBMITTED, undefined);

    editDraft(view, NEWER);
    await acknowledge(request, planFor(TASK_A, SUBMITTED));
    expect(view.result.current.draft.draftContent).toBe(NEWER);
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
    await advanceAutosave();
    expect(transport.create).toHaveBeenCalledTimes(1);
    expect(transport.update).toHaveBeenCalledExactlyOnceWith(TASK_A, NEWER, TITLE);
  });

  // @covers AC-TASKS-DOCUMENTS-003.3
  it("cleans an unchanged submitted draft without a redundant save and saves the next edit", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    editDraft(view, SUBMITTED);
    const editorKey = view.result.current.draft.editorKey;
    await advanceAutosave();
    await acknowledge(request, planFor(TASK_A, SUBMITTED));

    expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    await advanceAutosave(4);
    expect(transport.update).toHaveBeenCalledTimes(1);
    editDraft(view, NEWER);
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledTimes(2);
    expect(transport.update).toHaveBeenLastCalledWith(TASK_A, NEWER, TITLE);
  });

  // @covers AC-TASKS-DOCUMENTS-003.5
  it("keeps newer typing after an explicit save and autosaves it next", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    const pending = submitExplicitly(view, SUBMITTED);
    editDraft(view, NEWER);
    const editorKey = view.result.current.draft.editorKey;
    await acknowledge(request, planFor(TASK_A, SUBMITTED));
    await pending;

    expect(view.result.current.draft.draftContent).toBe(NEWER);
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledTimes(2);
    expect(transport.update).toHaveBeenLastCalledWith(TASK_A, NEWER, TITLE);
  });
});

describe("genuine external plan content", () => {
  // @covers AC-TASKS-DOCUMENTS-003.4
  it.each([false, true])("adopts an external update with a dirty draft: %s", async (dirty) => {
    const view = mountDraft();
    if (dirty) editDraft(view, NEWER);
    const editorKey = view.result.current.draft.editorKey;
    act(() =>
      view.result.current.store.getState().setTaskPlan(TASK_A, planFor(TASK_A, "Agent outline")),
    );
    expect(view.result.current.draft.draftContent).toBe("Agent outline");
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    expect(view.result.current.draft.editorKey).toBe(editorKey + 1);
    await advanceAutosave(4);
    expect(transport.update).not.toHaveBeenCalled();
  });

  // @covers AC-TASKS-DOCUMENTS-003.4
  it("adopts plan deletion without autosaving the outgoing draft", async () => {
    const view = mountDraft();
    editDraft(view, NEWER);
    act(() => view.result.current.store.getState().setTaskPlan(TASK_A, null));
    expect(view.result.current.draft.draftContent).toBe("");
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    await advanceAutosave(4);
    expect(transport.create).not.toHaveBeenCalled();
    expect(transport.update).not.toHaveBeenCalled();
  });

  // @covers AC-TASKS-DOCUMENTS-003.4
  it("does not hide a later external update matching an already consumed own acknowledgement", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    const pending = submitExplicitly(view, SUBMITTED);
    await acknowledge(request, planFor(TASK_A, SUBMITTED));
    await pending;
    editDraft(view, NEWER);
    await advanceAutosave();
    expect(view.result.current.saved.plan?.content).toBe(NEWER);

    editDraft(view, "Another unsaved local edit");
    act(() => view.result.current.store.getState().setTaskPlan(TASK_A, planFor(TASK_A, SUBMITTED)));
    expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    await advanceAutosave(4);
    expect(transport.update).toHaveBeenCalledTimes(2);
  });

  // @covers AC-TASKS-DOCUMENTS-003.4, AC-TASKS-DOCUMENTS-003.7
  it("does not hide an external update matching a failed own submission", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    const pending = submitExplicitly(view, SUBMITTED);
    await rejectSave(request, sizeRejection());
    await pending;
    editDraft(view, NEWER);

    act(() => view.result.current.store.getState().setTaskPlan(TASK_A, planFor(TASK_A, SUBMITTED)));
    expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    await advanceAutosave(4);
    expect(transport.update).toHaveBeenCalledTimes(1);
  });
});

describe("save failure compatibility", () => {
  // @covers AC-TASKS-DOCUMENTS-003.7
  it.each(["edit", "explicit"] as const)(
    "retains a size-rejected draft until an %s retry",
    async (retry) => {
      const request = deferredSave();
      transport.update.mockImplementationOnce(() => request.promise);
      const view = mountDraft();
      editDraft(view, SUBMITTED);
      await advanceAutosave();
      await rejectSave(request, sizeRejection());
      expect(view.result.current.saved.plan?.content).toBe(BASELINE);
      expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
      expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
      expect(view.result.current.saved.saveError?.kind).toBe("content-too-large");
      await advanceAutosave(4);
      expect(transport.update).toHaveBeenCalledTimes(1);

      if (retry === "edit") {
        editDraft(view, "Shorter outline");
        await advanceAutosave();
        expect(transport.update).toHaveBeenLastCalledWith(TASK_A, "Shorter outline", TITLE);
      } else {
        await act(async () => {
          await view.result.current.draft.attemptSave(SUBMITTED, TITLE);
        });
        expect(transport.update).toHaveBeenLastCalledWith(TASK_A, SUBMITTED, TITLE);
      }
      expect(transport.update).toHaveBeenCalledTimes(2);
      expect(view.result.current.saved.saveError).toBeNull();
      expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
    },
  );

  // @covers AC-TASKS-DOCUMENTS-003.7
  it("keeps an unchanged draft eligible for automatic retry after a generic failure", async () => {
    const request = deferredSave();
    transport.update.mockImplementationOnce(() => request.promise);
    const view = mountDraft();
    editDraft(view, SUBMITTED);
    await advanceAutosave();
    await rejectSave(request, new Error("Temporary transport failure"));
    expect(view.result.current.saved.plan?.content).toBe(BASELINE);
    expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
    expect(view.result.current.saved.saveError?.kind).toBe("generic");
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledTimes(2);
    expect(transport.update).toHaveBeenLastCalledWith(TASK_A, SUBMITTED, TITLE);
    expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
  });
});

describe("overlapping save compatibility", () => {
  // @covers AC-TASKS-DOCUMENTS-003.5
  it.each(["older-first", "latest-first"] as const)(
    "preserves newer typing across overlapping autosave and explicit success: %s",
    async (order) => {
      const first = deferredSave();
      const latest = deferredSave();
      transport.update
        .mockImplementationOnce(() => first.promise)
        .mockImplementationOnce(() => latest.promise);
      const view = mountDraft();
      editDraft(view, SUBMITTED);
      await advanceAutosave();
      expect(view.result.current.saved.isSaving).toBe(true);
      const latestPending = submitExplicitly(view, LATEST_SUBMITTED);
      editDraft(view, NEWER);
      const editorKey = view.result.current.draft.editorKey;

      if (order === "older-first") {
        await acknowledge(first, planFor(TASK_A, SUBMITTED));
        expect(view.result.current.saved.plan?.content).toBe(BASELINE);
        expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
        await acknowledge(latest, planFor(TASK_A, LATEST_SUBMITTED));
        await latestPending;
      } else {
        await acknowledge(latest, planFor(TASK_A, LATEST_SUBMITTED));
        await latestPending;
        await acknowledge(first, planFor(TASK_A, SUBMITTED));
      }
      expect(view.result.current.saved.plan?.content).toBe(LATEST_SUBMITTED);
      expect(view.result.current.draft.draftContent).toBe(NEWER);
      expect(view.result.current.draft.editorKey).toBe(editorKey);
      expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
      await advanceAutosave();
      expect(transport.update).toHaveBeenCalledTimes(3);
      expect(transport.update).toHaveBeenLastCalledWith(TASK_A, NEWER, TITLE);
    },
  );

  // @covers AC-TASKS-DOCUMENTS-003.5, AC-TASKS-DOCUMENTS-003.7
  it.each([SUBMITTED, "Different rejected outline"])(
    "does not let an older success clear a later size rejection for %s",
    async (latestContent) => {
      const first = deferredSave();
      const latest = deferredSave();
      transport.update
        .mockImplementationOnce(() => first.promise)
        .mockImplementationOnce(() => latest.promise);
      const view = mountDraft();
      const firstPending = submitExplicitly(view, SUBMITTED);
      const latestPending = submitExplicitly(view, latestContent);
      await rejectSave(latest, sizeRejection());
      await latestPending;
      await acknowledge(first, planFor(TASK_A, SUBMITTED));
      await firstPending;

      expect(view.result.current.saved.plan?.content).toBe(BASELINE);
      expect(view.result.current.draft.draftContent).toBe(latestContent);
      expect(view.result.current.draft.hasUnsavedChanges).toBe(true);
      expect(view.result.current.saved.saveError?.kind).toBe("content-too-large");
      await advanceAutosave(4);
      expect(transport.update).toHaveBeenCalledTimes(2);
    },
  );
});

describe("task view ownership", () => {
  // @covers AC-TASKS-DOCUMENTS-003.6
  it.each(["equal-plan", "no-plan", "no-task"] as const)(
    "resets the draft and pending debounce when switching to %s",
    async (destination) => {
      const view = mountDraft(destination === "no-plan" ? null : planFor(TASK_A, BASELINE));
      if (destination === "equal-plan") {
        act(() =>
          view.result.current.store.getState().setTaskPlan(TASK_B, planFor(TASK_B, BASELINE)),
        );
      }
      editDraft(view, NEWER);
      view.rerender({ taskId: destination === "no-task" ? null : TASK_B });
      expect(view.result.current.draft.draftContent).toBe(
        destination === "equal-plan" ? BASELINE : "",
      );
      expect(view.result.current.draft.hasUnsavedChanges).toBe(false);
      await advanceAutosave(4);
      expect(transport.create).not.toHaveBeenCalled();
      expect(transport.update).not.toHaveBeenCalled();
    },
  );

  // @covers AC-TASKS-DOCUMENTS-003.6, AC-TASKS-DOCUMENTS-003.7
  it("publishes a background success only to its task without clearing the new task's same-text rejection", async () => {
    const outgoing = deferredSave();
    const current = deferredSave();
    transport.update
      .mockImplementationOnce(() => outgoing.promise)
      .mockImplementationOnce(() => current.promise);
    const view = mountDraft();
    act(() =>
      view.result.current.store.getState().setTaskPlan(TASK_B, planFor(TASK_B, "Task B baseline")),
    );
    const outgoingPending = submitExplicitly(view, SUBMITTED);
    view.rerender({ taskId: TASK_B });
    const currentPending = submitExplicitly(view, SUBMITTED);
    await rejectSave(current, sizeRejection());
    await currentPending;
    const editorKey = view.result.current.draft.editorKey;
    await acknowledge(outgoing, planFor(TASK_A, SUBMITTED));
    await outgoingPending;

    expect(view.result.current.store.getState().taskPlans.byTaskId[TASK_A]?.content).toBe(
      SUBMITTED,
    );
    expect(view.result.current.saved.plan?.content).toBe("Task B baseline");
    expect(view.result.current.draft.draftContent).toBe(SUBMITTED);
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    expect(view.result.current.saved.saveError?.kind).toBe("content-too-large");
    await advanceAutosave(4);
    expect(transport.update).toHaveBeenCalledTimes(2);
  });

  // @covers AC-TASKS-DOCUMENTS-003.6
  it("drops a rejection from a previous task view after an A-to-B-to-A round trip", async () => {
    const outgoing = deferredSave();
    transport.update.mockImplementationOnce(() => outgoing.promise);
    const view = mountDraft();
    const pending = submitExplicitly(view, SUBMITTED);
    view.rerender({ taskId: TASK_B });
    view.rerender({ taskId: TASK_A });
    editDraft(view, "Fresh task A edit");
    const editorKey = view.result.current.draft.editorKey;
    await rejectSave(outgoing, sizeRejection());
    await pending;
    expect(view.result.current.saved.saveError).toBeNull();
    expect(view.result.current.draft.draftContent).toBe("Fresh task A edit");
    expect(view.result.current.draft.editorKey).toBe(editorKey);
    await advanceAutosave();
    expect(transport.update).toHaveBeenCalledTimes(2);
    expect(transport.update).toHaveBeenLastCalledWith(TASK_A, "Fresh task A edit", TITLE);
  });
});
