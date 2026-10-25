import { useTranslation } from "react-i18next";
import type { ConversationMatch, ConversationSearchState } from "@/lib/types/conversation-search";

export function ConversationExcerpt({ match }: { match?: ConversationMatch }) {
  const { t } = useTranslation();
  if (!match) return null;
  const text = Array.from(match.excerpt);
  const [start, end] = match.highlight_ranges[0] ?? [0, 0];
  return (
    <span className="w-full min-w-0 text-xs text-muted-foreground">
      <span className="mb-1 block text-[0.65rem]">{t("common:conversationSearch_match")}</span>
      <span className="line-clamp-2 whitespace-pre-wrap break-words">
        {text.slice(0, start).join("")}
        <mark className="rounded-sm bg-primary/15 text-foreground">
          {text.slice(start, end).join("")}
        </mark>
        {text.slice(end).join("")}
      </span>
    </span>
  );
}

export function ConversationSearchStatus({ state }: { state?: ConversationSearchState }) {
  const { t } = useTranslation();
  if (!state || state === "ready") return null;
  return (
    <p role="status" className="shrink-0 px-3 py-2 text-xs text-muted-foreground">
      {t(`common:conversationSearch_${state}`)}
    </p>
  );
}
