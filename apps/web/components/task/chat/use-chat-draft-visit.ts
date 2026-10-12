import { useCallback, useLayoutEffect, useMemo, useRef } from "react";
import { setChatDraftContent, setChatDraftText } from "@/lib/local-storage";
import type { FileAttachment } from "./file-attachment";

export function attachmentSnapshot(attachments: FileAttachment[]): string {
  return attachments.map((att) => `${att.id}:${att.deliveryMode ?? "prompt"}`).join("|");
}

export function clearDraftText(sessionId: string | null) {
  if (!sessionId) return;
  setChatDraftText(sessionId, "");
  setChatDraftContent(sessionId, null);
}

export function useDraftVisit(taskId: string | null, sessionId: string | null) {
  const owner = useMemo(() => ({ taskId, sessionId }), [taskId, sessionId]);
  const activeVisit = useRef<{ owner: typeof owner } | null>(null);
  useLayoutEffect(() => {
    const visit = { owner };
    activeVisit.current = visit;
    return () => {
      if (activeVisit.current === visit) activeVisit.current = null;
    };
  }, [owner]);
  return useCallback(
    () => (activeVisit.current?.owner === owner ? activeVisit.current : null),
    [owner],
  );
}
