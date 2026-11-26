import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { OpenFileTab } from "@/lib/types/backend";
import { treePath, isWithin } from "@/hooks/file-rename-continuity";
import {
  getActiveTabForSession,
  getOpenFileTabs,
  setActiveTabForSession,
  setOpenFileTabs,
} from "@/lib/local-storage";
import { getFileTabKey } from "./task-center-panel-file-tabs";

type FileIdentity = Pick<OpenFileTab, "path" | "repo"> & { resolvedPath?: string };
export function matchesTabletFileDeletion(file: FileIdentity, deleted: string): boolean {
  const path = treePath(file);
  const resolved = file.resolvedPath ? treePath({ path: file.resolvedPath }) : null;
  return (
    (path !== null && isWithin(path, deleted)) || (resolved !== null && isWithin(resolved, deleted))
  );
}

function matchesSelection(file: FileIdentity, selected: string): boolean {
  const workspace = treePath(file);
  return (
    selected === `file:${getFileTabKey(file)}` ||
    selected === `file:${file.path}` ||
    (workspace !== null && selected === `file:${getFileTabKey({ path: workspace })}`)
  );
}
/** Confirmed absence prunes saved descriptors immediately, independently of restoration completion. */
export function retireDeletedTabletFile(
  store: StoreApi<AppState>,
  sessionId: string,
  path: string,
  live: OpenFileTab[],
): OpenFileTab[] {
  const saved = getOpenFileTabs(sessionId);
  const removed = (file: FileIdentity) => matchesTabletFileDeletion(file, path);
  const retained = saved.filter((file) => !removed(file));
  if (retained.length !== saved.length) setOpenFileTabs(sessionId, retained);
  const removedSelection = (selected: string) =>
    live.some((file) => removed(file) && matchesSelection(file, selected)) ||
    saved.some((file) => removed(file) && matchesSelection(file, selected));
  const state = store.getState();
  const selected = state.tabletSession.activeTabBySessionId[sessionId];
  if (selected && removedSelection(selected)) state.setTabletSessionTab(sessionId, "chat");
  if (removedSelection(getActiveTabForSession(sessionId, "chat")))
    setActiveTabForSession(sessionId, "chat");
  return live.filter((file) => !removed(file));
}
