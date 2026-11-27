import { afterEach, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import {
  getActiveTabForSession,
  getOpenFileTabs,
  setActiveTabForSession,
  setOpenFileTabs,
} from "@/lib/local-storage";
import { retireDeletedTabletFile } from "./tablet-file-deletion";
import { getFileTabKey } from "./task-center-panel-file-tabs";
import { captureTabletFileOpen } from "@/lib/state/tablet-file-authority";
import type { OpenFileTab } from "@/lib/types/backend";
afterEach(() => sessionStorage.clear());
it.each(["file:repo\0F.txt", "file:\0repo/F.txt", "file:F.txt"])(
  "retires saved selection %s before any editor finishes restoring",
  (selection) => {
    const store = createAppStore();
    store.getState().setTabletSessionTab("B", selection);
    setActiveTabForSession("B", selection);
    setOpenFileTabs("B", [
      { path: "F.txt", name: "F.txt", repo: "repo" },
      { path: "F.txt", name: "F.txt", repo: "other" },
    ]);
    expect(retireDeletedTabletFile(store, "B", "repo/F.txt", [])).toEqual([]);
    expect(getOpenFileTabs("B")).toEqual([
      { path: "F.txt", name: "F.txt", repo: "other", pinned: true },
    ]);
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("chat");
    expect(getActiveTabForSession("B", "invalid")).toBe("chat");
  },
);
it("uses the current installed tabs and leaves a different pending file intent intact", () => {
  const store = createAppStore();
  const file: OpenFileTab = {
    path: "F.txt",
    name: "F.txt",
    content: "dirty",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  };
  const survivor = { ...file, path: "G.txt", name: "G.txt", content: "surviving draft" };
  store.getState().setTabletSessionTab("B", `file:${getFileTabKey(file)}`);
  const pending = captureTabletFileOpen(store, "B", "H.txt", () => true);
  expect(retireDeletedTabletFile(store, "B", "F.txt", [file, survivor])).toEqual([survivor]);
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("chat");
  expect(pending.current()).toBe(true);
  pending.finish();
});
