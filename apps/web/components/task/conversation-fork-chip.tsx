"use client";

import { Button } from "@kandev/ui/button";
import { IconEye, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ConversationForkFormContext } from "./conversation-fork-types";

export function ConversationForkChip({ fork }: { fork: ConversationForkFormContext }) {
  const { t } = useTranslation();
  const descriptor = fork.snapshot.descriptor;
  const startIndex = fork.source.boundaries.findIndex(
    (boundary) => boundary.message.id === fork.selection.startMessageId,
  );
  const selectedStart = startIndex >= 0 ? fork.source.boundaries[startIndex]?.message : undefined;
  const range = selectedStart
    ? t("task:conversationForkChipRangeStart", {
        role:
          selectedStart.author_type === "user"
            ? t("task:conversationForkUser")
            : t("task:conversationForkAssistant"),
        number: startIndex + 1,
      })
    : t("task:conversationForkChipRangeFull");
  const historyPreview = fork.snapshot.content.content.replace(/\s+/g, " ").trim().slice(0, 240);
  return (
    <section
      aria-label={t("task:conversationForkContext")}
      data-testid="conversation-fork-chip"
      className="group flex min-w-0 flex-wrap items-center gap-2 rounded-lg border bg-muted/30 p-2"
    >
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium">
          {t("task:conversationForkChipTitle", { title: fork.source.title })}
        </p>
        <p className="text-[11px] text-muted-foreground">
          {t("task:conversationForkChipDetails", {
            messages: descriptor.message_count,
            tokens: new Intl.NumberFormat().format(descriptor.estimate.estimated_tokens),
          })}
        </p>
        <p className="truncate text-[11px] text-muted-foreground">{range}</p>
      </div>
      <Button
        type="button"
        variant="outline"
        onClick={fork.onPreview}
        className="h-7 min-h-7 [@media(pointer:coarse)]:min-h-11"
      >
        <IconEye className="size-4" aria-hidden="true" />
        {t("task:conversationForkPreview")}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={fork.onRemove}
        aria-label={t("task:conversationForkRemove")}
        className="h-7 min-h-7 w-7 min-w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:min-w-11"
      >
        <IconX className="size-4" aria-hidden="true" />
      </Button>
      <p
        className="max-h-16 basis-full overflow-hidden text-xs text-muted-foreground transition-[max-height] duration-200 md:max-h-0 md:group-hover:max-h-16 md:group-focus-within:max-h-16"
        data-testid="conversation-fork-chip-history-preview"
      >
        <span className="font-medium">{t("task:conversationForkChipHistoryPreview")}: </span>
        {historyPreview}
      </p>
    </section>
  );
}
