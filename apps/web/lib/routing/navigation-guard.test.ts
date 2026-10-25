import { afterEach, expect, it, vi } from "vitest";
import {
  clearNavigationBlockerForTests,
  pushNavigationState,
  setNavigationBlocker,
  subscribeHistoryCommit,
} from "./navigation-guard";

afterEach(clearNavigationBlockerForTests);

it("publishes exact committed locations including silent native replacements", () => {
  window.history.replaceState({}, "", "/t/task-a?sessionId=first#plan");
  const listener = vi.fn();
  const unsubscribe = subscribeHistoryCommit(listener);
  window.history.replaceState({}, "", "/t/task-a?sessionId=second#changes");
  expect(listener).toHaveBeenLastCalledWith(
    "/t/task-a?sessionId=first#plan",
    "/t/task-a?sessionId=second#changes",
  );
  pushNavigationState({}, "", "/settings");
  expect(listener).toHaveBeenLastCalledWith("/t/task-a?sessionId=second#changes", "/settings");
  unsubscribe();
  window.history.pushState({}, "", "/stats");
  expect(listener).toHaveBeenCalledTimes(2);
});

it("does not publish rejected or failed history mutations", () => {
  window.history.replaceState({}, "", "/settings");
  const listener = vi.fn();
  const unsubscribe = subscribeHistoryCommit(listener);
  const unblock = setNavigationBlocker((intent) => intent.cancel());
  pushNavigationState({}, "", "/stats");
  expect(listener).not.toHaveBeenCalled();
  expect(() => window.history.pushState({}, "", "https://other.example/settings")).toThrow();
  expect(listener).not.toHaveBeenCalled();
  unblock();
  pushNavigationState({}, "", "/stats");
  expect(listener).toHaveBeenCalledOnce();
  unsubscribe();
});
