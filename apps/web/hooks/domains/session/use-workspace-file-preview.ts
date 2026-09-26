"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FileContentResponse } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";

export type WorkspaceFilePreviewState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "error" }
  | { kind: "loaded"; response: FileContentResponse & { is_binary: boolean } };

export function useWorkspaceFilePreview(
  sessionId: string | undefined,
  path: string,
  repo?: string,
) {
  const requestGeneration = useRef(0);
  const [state, setState] = useState<WorkspaceFilePreviewState>({ kind: "idle" });

  useEffect(() => {
    requestGeneration.current += 1;
    setState({ kind: "idle" });
    return () => {
      requestGeneration.current += 1;
    };
  }, [path, repo, sessionId]);

  const load = useCallback(async () => {
    const generation = (requestGeneration.current += 1);
    const client = getWebSocketClient();
    if (!client || !sessionId) {
      setState({ kind: "error" });
      return;
    }
    setState({ kind: "loading" });
    try {
      const response = await requestFileContent(client, sessionId, path, repo);
      if (requestGeneration.current === generation) setState({ kind: "loaded", response });
    } catch {
      if (requestGeneration.current === generation) setState({ kind: "error" });
    }
  }, [path, repo, sessionId]);

  return { load, state };
}
