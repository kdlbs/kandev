import type { OpenFileTab } from "@/lib/types/backend";

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

export function upsertOpenFileTab(prev: OpenFileTab[], fileTab: OpenFileTab): OpenFileTab[] {
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
          renderedPreview: fileTab.renderedPreview ?? existing.renderedPreview,
          markdownMode: fileTab.markdownMode ?? existing.markdownMode,
        };
    if (areOpenFileTabsEqual(existing, refreshed)) return prev;
    return prev.map((tab, index) => (index === existingIndex ? refreshed : tab));
  }
  const maxTabs = 4;
  return prev.length >= maxTabs ? [...prev.slice(1), fileTab] : [...prev, fileTab];
}
