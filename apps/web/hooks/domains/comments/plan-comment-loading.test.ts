import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { planCommentLoaderFor } from "./plan-comment-loading";

const api = vi.hoisted(() => ({ getTaskPlan: vi.fn() }));
vi.mock("@/lib/api/domains/plan-api", () => api);
const releases: Array<() => void> = [];

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
});
afterEach(() => {
  releases.splice(0).forEach((release) => release());
  vi.useRealTimers();
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
