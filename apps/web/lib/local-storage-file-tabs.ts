import type { StoredFileTab } from "./local-storage";
import { isMarkdownFile } from "./utils/file-types";

export function normalizeStoredFileTab(tab: StoredFileTab): StoredFileTab {
  const { markdownPreview, renderedPreview, ...current } = tab;
  const preview = isMarkdownFile(tab.path) ? (renderedPreview ?? markdownPreview) : undefined;
  return Object.assign(current, preview === undefined ? {} : { renderedPreview: preview });
}
