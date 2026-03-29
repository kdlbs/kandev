import { useCallback, useState } from "react";
import { executeUtilityPrompt } from "@/lib/api/domains/utility-api";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";

function formatTranscript(messages: Message[]): string {
  return messages
    .filter((m) => m.type === "message" || m.type === "content")
    .map((m) => {
      const role = m.author_type === "user" ? "User" : "Agent";
      return `${role}: ${m.content}`;
    })
    .join("\n\n");
}

export function useSummarizeSession() {
  const [isSummarizing, setIsSummarizing] = useState(false);

  const summarize = useCallback(async (sessionId: string): Promise<string | null> => {
    setIsSummarizing(true);
    try {
      // Fetch messages from API — they may not be in the store for non-active sessions
      const resp = await listTaskSessionMessages(sessionId, { sort: "asc" });
      const messages = resp.messages ?? [];
      if (!messages.length) return null;

      const transcript = formatTranscript(messages);
      if (!transcript) return null;

      const result = await executeUtilityPrompt({
        utility_agent_id: "builtin-summarize-session",
        session_id: sessionId,
        conversation_history: transcript,
      });
      return result.success ? (result.response ?? null) : null;
    } catch {
      return null;
    } finally {
      setIsSummarizing(false);
    }
  }, []);

  return { summarize, isSummarizing };
}
