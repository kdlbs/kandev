import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan, TaskPlanCommentSnapshot } from "@/lib/types/http";
import type { TextSelection } from "@/components/editors/tiptap/tiptap-plan-editor";

const mocks = vi.hoisted(() => ({
  touch: false,
  getTaskPlan: vi.fn(),
  getTaskPlanComments: vi.fn(),
  createTaskPlanComment: vi.fn(),
  updateTaskPlanComment: vi.fn(),
  deleteTaskPlanComment: vi.fn(),
}));
vi.mock("@/lib/api/domains/plan-api", () => ({
  getTaskPlan: mocks.getTaskPlan,
  listPlanRevisions: async () => [],
}));
vi.mock("@/lib/api/domains/plan-comment-api", () => mocks);
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => mocks.touch }));
vi.mock("./task-plan-panel-header", () => ({ PlanPanelHeader: () => null }));
vi.mock("@/hooks/domains/comments/use-plan-comment-migration", () => ({
  usePlanCommentMigration: () => ({ needsAttention: false }),
}));
vi.mock("@/lib/routing/client-dynamic", () => ({
  default:
    () =>
    ({
      onSelectionChange,
      onCommentClick,
    }: {
      onSelectionChange: (selection: TextSelection) => void;
      onCommentClick: (id: string, position: { x: number; y: number }) => void;
    }) => (
      <>
        <button
          onClick={() =>
            onSelectionChange({ text: "Plan step", from: 1, to: 10, position: { x: 20, y: 20 } })
          }
        >
          Select plan text
        </button>
        <button onClick={() => onCommentClick("comment-1", { x: 20, y: 20 })}>
          Edit saved comment
        </button>
      </>
    ),
}));

import { TaskPlanPanel } from "./task-plan-panel";

const TASK = "task-1";
const TIMESTAMP = "2026-09-13T00:00:00Z";
const DRAFT = "Unfinished feedback with\na second line";
const PLACEHOLDER = "Add your comment or instruction...";
const plan: TaskPlan = {
  id: "plan-1",
  task_id: TASK,
  title: "Plan",
  content: "Plan step",
  created_by: "agent",
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};
const snapshot: TaskPlanCommentSnapshot = {
  task_id: TASK,
  plan_id: plan.id,
  revision: 1,
  comments: [
    {
      id: "comment-1",
      task_id: TASK,
      plan_id: plan.id,
      body: "Saved feedback",
      selected_text: "Plan step",
      anchor_from: 1,
      anchor_to: 10,
      version: 1,
      created_at: TIMESTAMP,
      updated_at: TIMESTAMP,
    },
  ],
};
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function mountPanel(taskId = TASK) {
  return render(
    <StateProvider>
      <CaptureStore />
      <TaskPlanPanel taskId={taskId} />
    </StateProvider>,
  );
}
function openDraft(editing = false) {
  fireEvent.click(
    screen.getByRole("button", { name: editing ? "Edit saved comment" : "Select plan text" }),
  );
  const input = screen.getByPlaceholderText(PLACEHOLDER) as HTMLTextAreaElement;
  fireEvent.change(input, { target: { value: DRAFT } });
  return input;
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getTaskPlan.mockResolvedValue(plan);
  mocks.getTaskPlanComments.mockResolvedValue(snapshot);
});
afterEach(() => {
  cleanup();
  mocks.touch = false;
  vi.restoreAllMocks();
});

// @covers AC-TASKS-PLAN-COMMENTS-001.9, AC-TASKS-PLAN-COMMENTS-001.10
for (const touch of [false, true]) {
  describe(touch ? "phone comment refresh" : "desktop comment refresh", () => {
    for (const editing of [false, true]) {
      for (const failure of [false, true]) {
        it(`preserves ${editing ? "unsaved edits" : "a new comment"} through ${failure ? "failed" : "successful"} foreground refresh`, async () => {
          mocks.touch = touch;
          mountPanel();
          await act(async () => {
            store.getState().setTaskPlan(TASK, plan);
            store.getState().setTaskPlanComments(TASK, snapshot);
            store.getState().setConnectionStatus("connected");
          });
          const input = openDraft(editing);
          let resolve!: (value: TaskPlan) => void;
          let reject!: (error: Error) => void;
          mocks.getTaskPlan.mockImplementationOnce(
            () =>
              new Promise<TaskPlan>((yes, no) => {
                resolve = yes;
                reject = no;
              }),
          );
          const before = mocks.getTaskPlan.mock.calls.length;
          vi.spyOn(Date, "now").mockReturnValue(Date.now() + 1000);
          await act(async () => {
            window.dispatchEvent(new Event("focus"));
          });
          expect(mocks.getTaskPlan).toHaveBeenCalledTimes(before + 1);
          expect(store.getState().taskPlans.loadingByTaskId[TASK]).toBe(true);
          try {
            expect(screen.queryByPlaceholderText(PLACEHOLDER)).toBe(input);
            expect(input.value).toBe(DRAFT);
          } finally {
            await act(async () => {
              if (failure) reject(new Error("offline"));
              else resolve(plan);
            });
          }
          expect(screen.getByPlaceholderText(PLACEHOLDER)).toBe(input);
          expect(input.value).toBe(DRAFT);
          expect(screen.getByText(/Plan step/)).toBeTruthy();
          expect(mocks.createTaskPlanComment).not.toHaveBeenCalled();
          expect(mocks.updateTaskPlanComment).not.toHaveBeenCalled();
          const updated = {
            ...snapshot,
            revision: 2,
            comments: [{ ...snapshot.comments[0], body: DRAFT, version: 2 }],
          };
          mocks.createTaskPlanComment.mockImplementation(async ({ id }: { id: string }) => ({
            ...updated,
            comments: [...snapshot.comments, { ...updated.comments[0], id }],
          }));
          mocks.updateTaskPlanComment.mockResolvedValue(updated);
          await act(async () => {
            fireEvent.click(screen.getByRole("button", { name: editing ? "Update" : "Add" }));
          });
          const mutation = editing ? mocks.updateTaskPlanComment : mocks.createTaskPlanComment;
          expect(mutation).toHaveBeenCalledTimes(1);
          expect(mutation).toHaveBeenCalledWith(
            expect.objectContaining({ body: DRAFT, taskId: TASK, planId: plan.id }),
          );
          if (!editing) {
            expect(mutation).toHaveBeenCalledWith(
              expect.objectContaining({ selectedText: "Plan step", anchorFrom: 1, anchorTo: 10 }),
            );
          }
          expect(screen.queryByPlaceholderText(PLACEHOLDER)).toBeNull();
        });
      }
    }
  });
}

// @covers AC-TASKS-PLAN-COMMENTS-001.11
it("shows initial loading without a current plan", () => {
  mountPanel();
  act(() => store.getState().setTaskPlanLoading(TASK, true));
  expect(screen.queryByTestId("plan-panel")).toBeNull();
  expect(screen.getByText("Loading plan...")).toBeTruthy();
});

for (const replacement of [null, { ...plan, id: "plan-2" }]) {
  it(`clears the draft on confirmed plan ${replacement ? "replacement" : "deletion"}`, () => {
    mountPanel();
    act(() => {
      store.getState().setTaskPlan(TASK, plan);
    });
    openDraft();
    act(() => {
      store.getState().setTaskPlan(TASK, replacement);
    });
    expect(screen.queryByPlaceholderText(PLACEHOLDER)).toBeNull();
  });
}

it("does not carry a draft to a different task that is loading", () => {
  const view = mountPanel();
  act(() => {
    store.getState().setTaskPlan(TASK, plan);
  });
  openDraft();
  act(() => store.getState().setTaskPlanLoading("task-2", true));
  view.rerender(
    <StateProvider>
      <CaptureStore />
      <TaskPlanPanel taskId="task-2" />
    </StateProvider>,
  );
  expect(screen.queryByPlaceholderText(PLACEHOLDER)).toBeNull();
  expect(screen.queryByTestId("plan-panel")).toBeNull();
});
