import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskPlan, TaskPlanCommentSnapshot } from "@/lib/types/http";
import { planCommentLoaderFor } from "./plan-comment-loading";

const api = vi.hoisted(() => ({ getTaskPlan: vi.fn() }));
vi.mock("@/lib/api/domains/plan-api", () => api);
const comments = vi.hoisted(() => ({ getTaskPlanComments: vi.fn() }));
vi.mock("@/lib/api/domains/plan-comment-api", () => comments);
const releases: Array<() => void> = [];

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
});
afterEach(() => {
  releases.splice(0).forEach((release) => release());
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("releases loading flags when plan hydration invalidates an in-flight read", async () => {
  const taskId = "task-1";
  const plan: TaskPlan = {
    id: "plan-1",
    task_id: taskId,
    title: "Plan",
    content: "Step",
    created_by: "agent",
    created_at: "2026-09-02T00:00:00Z",
    updated_at: "2026-09-02T00:00:00Z",
  };
  const snapshot: TaskPlanCommentSnapshot = {
    task_id: taskId,
    plan_id: plan.id,
    revision: 1,
    comments: [],
  };
  const store = createAppStore();
  store.getState().setTaskPlan(taskId, plan);
  store.setState((state) => ({ taskPlans: { ...state.taskPlans, loadedByTaskId: {} } }));
  store.getState().setConnectionStatus("connected");
  const loader = planCommentLoaderFor(store, taskId, "Could not load comments");
  releases.push(loader.attach());
  let resolve!: (value: TaskPlanCommentSnapshot) => void;
  comments.getTaskPlanComments.mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const reading = loader.load(false);
  await vi.advanceTimersByTimeAsync(0);
  expect(store.getState().taskPlans.commentsLoadingByTaskId[taskId]).toBe(true);
  store.getState().setTaskPlan(taskId, plan);
  resolve(snapshot);
  await reading;
  expect(store.getState().taskPlans.commentsLoadingByTaskId[taskId]).toBe(false);
  expect(store.getState().taskPlans.loadingByTaskId[taskId]).toBe(false);
  expect(store.getState().taskPlans.commentsByTaskId[taskId]).toBeUndefined();
  comments.getTaskPlanComments.mockResolvedValue(snapshot);
  await loader.load(false);
  expect(store.getState().taskPlans.commentsByTaskId[taskId]).toEqual(snapshot);
});

it("does not let a hidden wake suppress immediate visible recovery", async () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const loader = planCommentLoaderFor(store, "task-1", "Could not load comments");
  releases.push(loader.attach());
  api.getTaskPlan.mockRejectedValueOnce(new Error("offline")).mockResolvedValue(null);
  await loader.load(true);
  const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  document.dispatchEvent(new Event("visibilitychange"));
  await loader.wake();
  visibility.mockReturnValue("visible");
  await loader.wake();
  expect(store.getState().taskPlans.commentsErrorByTaskId["task-1"]).toBeUndefined();
  expect(api.getTaskPlan).toHaveBeenCalledTimes(2);
});

it("uses the current localized error when a retained loader fails again", async () => {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const loader = planCommentLoaderFor(store, "task-1", "Old localized error");
  releases.push(loader.attach());
  api.getTaskPlan.mockRejectedValue(new Error("offline"));
  await loader.load(true);
  const translated = planCommentLoaderFor(store, "task-1", "Current localized error");
  expect(translated).toBe(loader);
  await translated.load(true);
  expect(store.getState().taskPlans.commentsErrorByTaskId["task-1"]).toBe(
    "Current localized error",
  );
});

it("resumes failed ordinary reads when a cached task surface remounts", async () => {
  const store = createAppStore();
  store.getState().setTaskPlan("task-1", null);
  store.getState().setConnectionStatus("connected");
  const loader = planCommentLoaderFor(store, "task-1", "Could not load comments");
  api.getTaskPlan.mockRejectedValueOnce(new Error("offline")).mockResolvedValue(null);
  const detach = loader.attach();
  await loader.load(true);
  detach();
  await vi.advanceTimersByTimeAsync(120000);
  expect(api.getTaskPlan).toHaveBeenCalledOnce();
  releases.push(loader.attach());
  await vi.advanceTimersByTimeAsync(0);
  expect(api.getTaskPlan).toHaveBeenCalledTimes(2);
  expect(store.getState().taskPlans.commentsErrorByTaskId["task-1"]).toBeUndefined();
});
