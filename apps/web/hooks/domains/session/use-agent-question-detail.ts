import { useCallback, useEffect, useState } from "react";
import { getAgentQuestion, refreshAgentQuestion } from "@/lib/api/domains/agent-questions-api";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import { getWebSocketClient } from "@/lib/ws/connection";

type DetailState = {
  question: ClarificationInboxAsyncQuestion | null;
  status: "loading" | "ready" | "error";
};

export function useAgentQuestionDetail(questionId: string | null) {
  const [state, setState] = useState<DetailState>({ question: null, status: "loading" });

  const read = useCallback(
    async (refresh: boolean) => {
      if (!questionId) {
        setState({ question: null, status: "error" });
        return;
      }
      try {
        const question = refresh
          ? await refreshAgentQuestion(questionId)
          : await getAgentQuestion(questionId);
        setState({ question, status: "ready" });
      } catch {
        setState((current) => ({ question: current.question, status: "error" }));
      }
    },
    [questionId],
  );

  useEffect(() => {
    let active = true;
    const load = async (refresh: boolean) => {
      if (!questionId) return;
      try {
        const question = refresh
          ? await refreshAgentQuestion(questionId)
          : await getAgentQuestion(questionId);
        if (active) setState({ question, status: "ready" });
      } catch {
        if (active) setState((current) => ({ question: current.question, status: "error" }));
      }
    };

    if (!questionId) {
      setState({ question: null, status: "error" });
      return () => {
        active = false;
      };
    }

    setState((current) => ({ question: current.question, status: "loading" }));
    void load(false);
    const client = getWebSocketClient();
    const unsubscribe = client?.on("task.question.changed", (message) => {
      if (message.payload.question_id === questionId) void load(true);
    });
    return () => {
      active = false;
      unsubscribe?.();
    };
  }, [questionId]);

  return { ...state, refresh: () => read(true) };
}
