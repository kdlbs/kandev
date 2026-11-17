import { useEffect, useState } from "react";
import { getOpenFileTabs, setOpenFileTabs } from "@/lib/local-storage";
export function useFileViewerChoice(sessionId: string, path: string, repo?: string) {
  const read = () =>
    getOpenFileTabs(sessionId).find((tab) => tab.path === path && tab.repo === repo)?.fileViewer ??
    null;
  const [choice, setChoice] = useState<string | null>(read);
  useEffect(() => {
    setChoice(
      getOpenFileTabs(sessionId).find((tab) => tab.path === path && tab.repo === repo)
        ?.fileViewer ?? null,
    );
  }, [sessionId, path, repo]);
  const select = (value: string) => {
    setChoice(value);
    const tabs = getOpenFileTabs(sessionId);
    const found = tabs.some((tab) => tab.path === path && tab.repo === repo);
    const next = tabs.map((tab) =>
      tab.path === path && tab.repo === repo ? { ...tab, fileViewer: value } : tab,
    );
    if (!found)
      next.push({
        path,
        repo,
        name: path.split("/").pop() ?? path,
        fileViewer: value,
        pinned: false,
      });
    setOpenFileTabs(sessionId, next);
  };
  return [choice, select] as const;
}
