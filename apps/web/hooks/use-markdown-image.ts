import { useEffect, useState } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { getImageMimeType } from "@/lib/utils/file-types";

type ImageState = { key: string; src?: string; error?: boolean };

// This map shares active reads only; completed image contents are never retained.
const pendingImageReads = new WeakMap<
  WebSocketClient,
  Map<string, ReturnType<typeof requestFileContent>>
>();

function requestMarkdownImage(client: WebSocketClient, sessionId: string, path: string) {
  const reads =
    pendingImageReads.get(client) ?? new Map<string, ReturnType<typeof requestFileContent>>();
  pendingImageReads.set(client, reads);

  const key = JSON.stringify([sessionId, path]);
  const existing = reads.get(key);
  if (existing) return existing;

  const pending = requestFileContent(client, sessionId, path).finally(() => {
    if (reads.get(key) !== pending) return;
    reads.delete(key);
    if (reads.size === 0) pendingImageReads.delete(client);
  });
  reads.set(key, pending);
  return pending;
}

export function useMarkdownImage(sessionId: string | null | undefined, path: string | null) {
  const client = useWebSocketClient();
  const key = JSON.stringify([sessionId, path]);
  const [state, setState] = useState<ImageState>({ key });
  useEffect(() => {
    if (!client || !sessionId || !path) return;
    const mime = getImageMimeType(path);
    if (!mime.startsWith("image/")) {
      setState({ key, error: true });
      return;
    }
    let active = true;
    void requestMarkdownImage(client, sessionId, path)
      .then((response) => {
        if (!active) return;
        const src = response.is_binary
          ? `data:${mime};base64,${response.content}`
          : `data:${mime};charset=utf-8,${encodeURIComponent(response.content)}`;
        setState({ key, src });
      })
      .catch(() => {
        if (active) setState({ key, error: true });
      });
    return () => {
      active = false;
    };
  }, [client, key, path, sessionId]);
  return state.key === key ? state : { key };
}
