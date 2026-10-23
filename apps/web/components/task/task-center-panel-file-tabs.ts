import { useCallback, useLayoutEffect, useMemo, type Dispatch, type SetStateAction } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import type { OpenFileTab } from "@/lib/types/backend";

import type { StoredFileTab } from "@/lib/local-storage";
import { getFilePreviewKind } from "@/lib/utils/file-types";
import { defaultMarkdownFileMode } from "./markdown-file-mode";

export type FileEditorTab = OpenFileTab & { instanceId?: symbol };

export function installFileEditorTab(tab: OpenFileTab): FileEditorTab {
  return {
    ...tab,
    markdownMode: tab.markdownMode ?? defaultMarkdownFileMode(tab.path),
    instanceId: Symbol(),
  };
}

export function toStoredFileTab({
  path,
  name,
  repo,
  renderedPreview,
  markdownMode,
}: OpenFileTab): StoredFileTab {
  const resolvedMode = markdownMode ?? defaultMarkdownFileMode(path);
  return {
    path,
    name,
    repo,
    ...(getFilePreviewKind(path) === "markdown" && renderedPreview ? { renderedPreview } : {}),
    ...(resolvedMode ? { markdownMode: resolvedMode } : {}),
  };
}

export function getFileTabKey(file: Pick<OpenFileTab, "path" | "repo">): string {
  return `${file.repo ?? ""}\u0000${file.path}`;
}

const openFileTabKeys: (keyof OpenFileTab)[] = [
  "resolvedPath",
  "path",
  "name",
  "repo",
  "content",
  "originalContent",
  "originalHash",
  "isDirty",
  "isBinary",
  "renderedPreview",
  "markdownMode",
];

function areOpenFileTabsEqual(left: OpenFileTab, right: OpenFileTab): boolean {
  return openFileTabKeys.every((key) => Object.is(left[key], right[key]));
}

export function upsertOpenFileTab(prev: FileEditorTab[], fileTab: OpenFileTab): FileEditorTab[] {
  const fileKey = getFileTabKey(fileTab);
  const existingIndex = prev.findIndex((tab) => getFileTabKey(tab) === fileKey);
  if (existingIndex >= 0) {
    const existing = prev[existingIndex];
    const refreshed = existing.isDirty
      ? {
          ...existing,
          resolvedPath: fileTab.resolvedPath,
          ...(fileTab.renderedPreview !== undefined
            ? { renderedPreview: fileTab.renderedPreview }
            : {}),
        }
      : {
          ...existing,
          ...fileTab,
          instanceId: existing.instanceId,
          renderedPreview: fileTab.renderedPreview ?? existing.renderedPreview,
          markdownMode: fileTab.markdownMode ?? existing.markdownMode,
        };
    if (areOpenFileTabsEqual(existing, refreshed)) return prev;
    return prev.map((tab, index) => (index === existingIndex ? refreshed : tab));
  }
  const maxTabs = 4;
  const installed = installFileEditorTab(fileTab);
  const cleanIndex = prev.length >= maxTabs ? prev.findIndex((tab) => !tab.isDirty) : -1;
  const retained = cleanIndex >= 0 ? prev.filter((_, index) => index !== cleanIndex) : prev;
  return [...retained, installed];
}

/** Tablet tabs project canonical records instead of owning editable copies. */
export function useCanonicalFileTabs(sessionId: string | null) {
  const files = useDockviewStore((state) => state.fileDrafts.get(sessionId ?? ""));
  const tabs = useMemo(() => Array.from(files?.values() ?? []), [files]);
  useLayoutEffect(() => {
    useDockviewStore.getState().activateFileSession(sessionId);
  }, [sessionId]);
  const setTabs = useCallback<Dispatch<SetStateAction<OpenFileTab[]>>>(
    (update) => {
      const store = useDockviewStore.getState();
      if (store.activeFileSessionId !== sessionId) return;
      if (typeof update !== "function") {
        for (const tab of update)
          store.ensureFileState(buildRepoScopedItemId(tab.path, tab.repo), tab);
        return;
      }
      const previous = Array.from(store.openFiles.values());
      const next = update(previous);
      const keys = new Set(next.map((tab) => buildRepoScopedItemId(tab.path, tab.repo)));
      for (const tab of previous) {
        const key = buildRepoScopedItemId(tab.path, tab.repo);
        if (!keys.has(key)) store.removeFileState(key);
      }
      for (const tab of next) {
        const key = buildRepoScopedItemId(tab.path, tab.repo);
        const current = useDockviewStore.getState().openFiles.get(key);
        if (current === tab) continue;
        if (current) store.updateFileState(key, tab);
        else store.ensureFileState(key, tab);
        store.selectFileDraft(key);
      }
    },
    [sessionId],
  );
  return [tabs, setTabs] as const;
}
