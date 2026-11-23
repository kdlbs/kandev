import { afterEach, expect, it, vi } from "vitest";
const state = vi.hoisted(() => ({
  api: null as object | null,
  isRestoringLayout: false,
  currentLayoutEnvId: "execution",
  preMaximizeLayout: null,
  persist: vi.fn(),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: { getState: () => state },
  persistEnvLayoutNow: state.persist,
}));
import { rememberFileViewerReturn, takeFileViewerReturn } from "./file-viewer-return";
afterEach(() => {
  state.api = null;
  vi.useRealTimers();
});
it("retains one-use phone file metadata scoped to the original session", () => {
  rememberFileViewerReturn("session", "model.stl", "repo");
  expect(takeFileViewerReturn("other")).toBeNull();
  expect(takeFileViewerReturn("session")).toEqual({ path: "model.stl", repo: "repo" });
  expect(takeFileViewerReturn("session")).toBeNull();
});
it("expires an abandoned return without restoring a selection", () => {
  vi.useFakeTimers();
  rememberFileViewerReturn("session", "model.stl");
  vi.advanceTimersByTime(300001);
  expect(takeFileViewerReturn("session")).toBeNull();
});
it("flushes the live desktop layout before discovery navigation", () => {
  state.api = {};
  rememberFileViewerReturn("session", "model.stl");
  expect(state.persist).toHaveBeenCalledWith(state.api, "execution", null);
  expect(takeFileViewerReturn("session")).toBeNull();
});
