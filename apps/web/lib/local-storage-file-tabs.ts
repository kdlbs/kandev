import type { StoredFileTab } from "./local-storage";
import { resolveStoredMarkdownFileMode } from "@/components/task/markdown-file-mode";
import { isMarkdownFile } from "./utils/file-types";

export function normalizeStoredFileTab(tab: StoredFileTab): StoredFileTab {
  const { markdownMode, markdownPreview, renderedPreview, ...current } = tab;
  if (!isMarkdownFile(tab.path)) return current;
  const mode = resolveStoredMarkdownFileMode({ markdownMode, markdownPreview, renderedPreview });
  return { ...current, markdownMode: mode };
}
