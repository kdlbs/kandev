"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  formatAgentMessageCommentsAsMarkdown,
  officeTaskCommentScope,
  useCommentsStore,
  type AgentMessageComment,
} from "@/lib/state/slices/comments";
import { usePendingAgentMessageComments } from "@/hooks/domains/comments/use-pending-comments";
import type { ContextItem } from "@/lib/types/context";

const CHIP_QUOTE_MAX_LENGTH = 40;

function shortQuote(text: string): string {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > CHIP_QUOTE_MAX_LENGTH ? `${flat.slice(0, CHIP_QUOTE_MAX_LENGTH)}…` : flat;
}

/** Prepends the formatted extracts block to the user's text; either part may be empty. */
export function composeCommentBody(extracts: AgentMessageComment[], text: string): string {
  const block = formatAgentMessageCommentsAsMarkdown(extracts);
  return block ? `${block}\n${text.trim()}`.trimEnd() : text.trim();
}

/** Pending extracts of an Office task thread, exposed as composer context chips. */
export function useThreadExtracts(taskId: string) {
  const { t } = useTranslation();
  const scope = officeTaskCommentScope(taskId);
  const hydrateSession = useCommentsStore((state) => state.hydrateSession);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const markCommentsSent = useCommentsStore((state) => state.markCommentsSent);
  const extracts = usePendingAgentMessageComments(scope);

  useEffect(() => hydrateSession(scope), [hydrateSession, scope]);

  const items = useMemo<ContextItem[]>(
    () =>
      extracts.map((extract) => ({
        kind: "agent-message-comment" as const,
        id: extract.id,
        label: t("task:threadExtractChipLabel", {
          author: extract.authorName ?? "",
          quote: shortQuote(extract.selectedText),
        }),
        comments: [extract],
        onRemove: () => removeComment(extract.id),
      })),
    [extracts, removeComment, t],
  );
  const markSent = useCallback(
    (sent: AgentMessageComment[]) => markCommentsSent(sent.map((extract) => extract.id)),
    [markCommentsSent],
  );
  return { extracts, items, markSent };
}
