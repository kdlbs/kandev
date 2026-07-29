import type { OpenFileTab } from "@/lib/types/backend";

export function upsertOpenFileTab(prev: OpenFileTab[], fileTab: OpenFileTab): OpenFileTab[] {
  const existingIndex = prev.findIndex((tab) => tab.path === fileTab.path);
  if (existingIndex >= 0) {
    if (fileTab.markdownPreview === undefined) return prev;
    return prev.map((tab, index) =>
      index === existingIndex ? { ...tab, markdownPreview: fileTab.markdownPreview } : tab,
    );
  }
  const maxTabs = 4;
  return prev.length >= maxTabs ? [...prev.slice(1), fileTab] : [...prev, fileTab];
}
