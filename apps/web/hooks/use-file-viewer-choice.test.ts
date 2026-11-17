import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { getOpenFileTabs, setOpenFileTabs } from "@/lib/local-storage";
import { useFileViewerChoice } from "./use-file-viewer-choice";
beforeEach(() => localStorage.clear());
it("keeps a viewer choice when tab persistence rewrites the tab", () => {
  const tab = { path: "model.stl", name: "model.stl", pinned: true, repo: "one" };
  setOpenFileTabs("session", [tab]);
  const hook = renderHook(() => useFileViewerChoice("session", tab.path, tab.repo));
  act(() => hook.result.current[1]("plugin/stl"));
  setOpenFileTabs("session", [tab]);
  expect(getOpenFileTabs("session")[0].fileViewer).toBe("plugin/stl");
  hook.unmount();
  expect(
    renderHook(() => useFileViewerChoice("session", tab.path, tab.repo)).result.current[0],
  ).toBe("plugin/stl");
  expect(
    renderHook(() => useFileViewerChoice("other", tab.path, tab.repo)).result.current[0],
  ).toBeNull();
  expect(
    renderHook(() => useFileViewerChoice("session", tab.path, "two")).result.current[0],
  ).toBeNull();
});
