import type { StoredFileTab } from "./local-storage";

export function normalizeStoredFileTab(tab: StoredFileTab): StoredFileTab {
  const { markdownPreview, ...current } = tab;
  return Object.assign(
    current,
    current.renderedPreview === undefined && markdownPreview !== undefined
      ? { renderedPreview: markdownPreview }
      : {},
  );
}
