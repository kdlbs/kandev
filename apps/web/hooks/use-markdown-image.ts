import { useEffect, useState } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { getImageMimeType } from "@/lib/utils/file-types";

type ImageState = { key: string; src?: string; error?: boolean };

export function useMarkdownImage(sessionId: string | null | undefined, path: string | null) {
  const client = useWebSocketClient();
  const key = JSON.stringify([sessionId, path]);
  const [state, setState] = useState<ImageState>({ key });
  useEffect(() => {
    if (!client || !sessionId || !path) return;
    const mime = getImageMimeType(path);
    if (!mime.startsWith("image/")) return;
    let active = true;
    void requestFileContent(client, sessionId, path)
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
