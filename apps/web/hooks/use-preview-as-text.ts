import { useCallback, useEffect, useRef, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { buildFileEditorState } from "./file-editor-state";
import type { FileEditorState } from "@/lib/state/dockview-store";
export function usePreviewAsText(path: string, repo: string | undefined, sessionId: string) {
  const [file, setFile] = useState<FileEditorState | null>(null);
  const [failed, setFailed] = useState(false);
  const [loading, setLoading] = useState(false);
  const generation = useRef<symbol>(Symbol());
  const clear = useCallback(() => {
    generation.current = Symbol();
    setFile(null);
    setFailed(false);
    setLoading(false);
  }, []);
  useEffect(() => {
    clear();
    return () => {
      generation.current = Symbol();
    };
  }, [path, repo, sessionId, clear]);
  const load = useCallback(async () => {
    const client = getWebSocketClient();
    if (!client) return;
    const request = generation.current;
    setLoading(true);
    setFailed(false);
    try {
      const response = await requestFileContent(client, sessionId, path, repo);
      if (response.is_binary) throw new Error();
      const next = await buildFileEditorState(path, response, repo);
      if (generation.current === request) setFile(next);
    } catch {
      if (generation.current === request) setFailed(true);
    } finally {
      if (generation.current === request) setLoading(false);
    }
  }, [path, repo, sessionId]);
  return { file, failed, loading, load, clear };
}
