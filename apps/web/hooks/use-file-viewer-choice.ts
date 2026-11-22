import { useCallback, useEffect, useState } from "react";
import { getOpenFileTabs, setOpenFileTabs } from "@/lib/local-storage";
export function useFileViewerChoice(sessionId: string, path: string, repo?: string) {
  const read = useCallback(
    () =>
      readViewerPreference(sessionId, path, repo) ??
      getOpenFileTabs(sessionId).find((tab) => tab.path === path && tab.repo === repo)
        ?.fileViewer ??
      null,
    [sessionId, path, repo],
  );
  const [choice, setChoice] = useState<string | null>(read);
  useEffect(() => {
    setChoice(read());
  }, [read]);
  const select = (value: string) => {
    setChoice(value);
    try {
      window.sessionStorage.setItem(viewerPreferenceKey(sessionId, path, repo), value);
    } catch {
      /* A preference can remain local when storage is unavailable. */
    }
    const tabs = getOpenFileTabs(sessionId);
    const found = tabs.some((tab) => tab.path === path && tab.repo === repo);
    const next = tabs.map((tab) =>
      tab.path === path && tab.repo === repo ? { ...tab, fileViewer: value } : tab,
    );
    if (found) setOpenFileTabs(sessionId, next);
  };
  return [choice, select] as const;
}

function viewerPreferenceKey(sessionId: string, path: string, repo?: string) {
  return `kandev.file-viewer-choice.${JSON.stringify([sessionId, repo ?? null, path])}`;
}
function readViewerPreference(sessionId: string, path: string, repo?: string) {
  try {
    return window.sessionStorage.getItem(viewerPreferenceKey(sessionId, path, repo));
  } catch {
    return null;
  }
}
