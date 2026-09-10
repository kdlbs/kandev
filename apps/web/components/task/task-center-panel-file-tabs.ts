import type { OpenFileTab } from "@/lib/types/backend";

export function getFileTabKey(file: Pick<OpenFileTab, "path" | "repo">): string {
  return `${file.repo ?? ""}\u0000${file.path}`;
}

export function upsertOpenFileTab(prev: OpenFileTab[], fileTab: OpenFileTab): OpenFileTab[] {
  const fileKey = getFileTabKey(fileTab);
  const existingIndex = prev.findIndex((tab) => getFileTabKey(tab) === fileKey);
  if (existingIndex >= 0) {
    if (fileTab.renderedPreview === undefined) return prev;
    return prev.map((tab, index) =>
      index === existingIndex ? { ...tab, renderedPreview: fileTab.renderedPreview } : tab,
    );
  }
  const maxTabs = 4;
  return prev.length >= maxTabs ? [...prev.slice(1), fileTab] : [...prev, fileTab];
}
