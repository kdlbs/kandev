"use client";

import { useState, useEffect } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";

type LazyFilePreviewProps = {
  path: string;
  sessionId: string | null;
};

export function LazyFilePreview({ path, sessionId }: LazyFilePreviewProps) {
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!sessionId) {
      setLoading(false);
      setError(true);
      return;
    }

    let cancelled = false;

    async function fetch() {
      try {
        const client = getWebSocketClient();
        if (!client) {
          setError(true);
          return;
        }
        const response = await requestFileContent(client, sessionId!, path);
        if (!cancelled) {
          setContent(response.content ?? null);
        }
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetch();
    return () => {
      cancelled = true;
    };
  }, [path, sessionId]);

  if (loading) {
    return (
      <div className="space-y-1.5">
        <div className="text-muted-foreground text-xs font-medium truncate">{path}</div>
        <div className="h-3 w-3/4 bg-muted animate-pulse rounded" />
        <div className="h-3 w-1/2 bg-muted animate-pulse rounded" />
      </div>
    );
  }

  if (error || content === null) {
    return <div className="text-muted-foreground text-xs">{path}</div>;
  }

  const preview = content.length > 2000 ? content.slice(0, 2000) + "..." : content;

  return (
    <div className="space-y-1.5">
      <div className="text-muted-foreground text-xs font-medium truncate">{path}</div>
      <pre className="text-[10px] leading-tight font-mono whitespace-pre-wrap break-all">
        {preview}
      </pre>
    </div>
  );
}
