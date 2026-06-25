import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { executeUtilityPrompt } from "@/lib/api/domains/utility-api";
import { sessionMessagesQueryOptions } from "@/lib/query/query-options";
import type { Message } from "@/lib/types/http";

export type SummarizeSessionResult = {
  summary: string | null;
  error?: string;
};

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
  const queryClient = useQueryClient();
  const [isSummarizing, setIsSummarizing] = useState(false);

  const summarize = useCallback(
    async (sessionId: string): Promise<SummarizeSessionResult> => {
      setIsSummarizing(true);
      try {
        // Fetch messages from API — they may not be in the store for non-active sessions
        const resp = await queryClient.fetchQuery(
          sessionMessagesQueryOptions(sessionId, { sort: "asc" }),
        );
        const messages = resp.messages ?? [];
        if (!messages.length) return { summary: null };

        const transcript = formatTranscript(messages);
        if (!transcript) return { summary: null };

        // Sessionless: handoff often runs against a completed session whose
        // agentctl is gone. Host utility executes the builtin summarize agent.
        const result = await executeUtilityPrompt({
          utility_agent_id: "builtin-summarize-session",
          conversation_history: transcript,
        });
        if (!result.success) {
          return { summary: null, error: result.error || "Summarize utility returned no result" };
        }
        return { summary: result.response ?? null };
      } catch (error) {
        return {
          summary: null,
          error: error instanceof Error ? error.message : "Could not generate a summary",
        };
      } finally {
        setIsSummarizing(false);
      }
    },
    [queryClient],
  );

  return { summarize, isSummarizing };
}
