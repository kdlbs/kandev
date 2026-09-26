"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconArrowLeft, IconEye, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import type { ConversationForkSelection } from "@/hooks/domains/task/use-conversation-fork";

type ConversationForkPreviewProps = {
  fork: ConversationForkFormContext;
  onBack: () => void;
};

function ForkRangeSelect({
  fork,
  selection,
  onSelectionChange,
}: {
  fork: ConversationForkFormContext;
  selection: ConversationForkSelection;
  onSelectionChange: (next: ConversationForkSelection) => void;
}) {
  const { t } = useTranslation();
  const finalized = fork.source.boundaries.filter((boundary) => boundary.finalized);
  return (
    <label className="grid gap-1.5 text-sm">
      <span className="font-medium">{t("task:conversationForkRange")}</span>
      <select
        aria-label={t("task:conversationForkRange")}
        value={selection.startMessageId ?? ""}
        onChange={(event) => {
          const startMessageId = event.currentTarget.value || undefined;
          fork.onRangeStartChange(startMessageId);
          onSelectionChange({ ...selection, startMessageId, attachmentIds: [] });
        }}
        className="h-7 min-h-7 w-full rounded-md border bg-background px-3 text-sm [@media(pointer:coarse)]:min-h-11"
      >
        <option value="">{t("task:conversationForkFullRange")}</option>
        {finalized.map(({ message }, index) => (
          <option key={message.id} value={message.id}>
            {t("task:conversationForkStartAt", {
              role:
                message.author_type === "user"
                  ? t("task:conversationForkUser")
                  : t("task:conversationForkAssistant"),
              number: index + 1,
            })}
          </option>
        ))}
      </select>
    </label>
  );
}

function ForkPreviewSelection({
  fork,
  selection,
  onSelectionChange,
  unavailableCount,
}: {
  fork: ConversationForkFormContext;
  selection: ConversationForkSelection;
  onSelectionChange: (next: ConversationForkSelection) => void;
  unavailableCount: number;
}) {
  const { t } = useTranslation();
  const availableAttachments = fork.source.attachments.filter((attachment) => attachment.available);

  return (
    <div className="grid gap-4 rounded-md border p-3">
      <ForkRangeSelect fork={fork} selection={selection} onSelectionChange={onSelectionChange} />
      <label className="flex min-h-7 items-center justify-between gap-4 text-sm [@media(pointer:coarse)]:min-h-11">
        <span>{t("task:conversationForkIncludeToolEvidence")}</span>
        <input
          type="checkbox"
          aria-label={t("task:conversationForkIncludeToolEvidence")}
          checked={selection.includeToolEvidence}
          onChange={(event) =>
            onSelectionChange({
              ...selection,
              includeToolEvidence: event.currentTarget.checked,
            })
          }
          className="size-5 accent-primary"
        />
      </label>
      <div className="grid gap-2">
        <h3 className="text-sm font-medium">{t("task:conversationForkAttachments")}</h3>
        {fork.attachmentsLoading && (
          <p role="status" className="text-xs text-muted-foreground">
            {t("task:conversationForkPreparing")}
          </p>
        )}
        {!fork.attachmentsLoading && availableAttachments.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("task:conversationForkNoAttachments")}</p>
        )}
        {!fork.attachmentsLoading &&
          availableAttachments.map((attachment) => {
            const id = attachment.source_id ?? attachment.id ?? "";
            return (
              <label
                key={id}
                className="flex min-h-7 items-center gap-3 text-sm [@media(pointer:coarse)]:min-h-11"
              >
                <input
                  type="checkbox"
                  aria-label={attachment.name}
                  checked={selection.attachmentIds.includes(id)}
                  onChange={(event) =>
                    onSelectionChange({
                      ...selection,
                      attachmentIds: event.currentTarget.checked
                        ? [...selection.attachmentIds, id]
                        : selection.attachmentIds.filter((selected) => selected !== id),
                    })
                  }
                  className="size-5 accent-primary"
                />
                <span className="min-w-0 flex-1 truncate">{attachment.name}</span>
                {typeof attachment.size === "number" && (
                  <span className="text-xs text-muted-foreground">{attachment.size} B</span>
                )}
              </label>
            );
          })}
        {unavailableCount > 0 && (
          <p className="text-xs text-muted-foreground">
            {t("task:conversationForkUnavailableAttachments", { count: unavailableCount })}
          </p>
        )}
      </div>
    </div>
  );
}

function ForkPreviewSnapshot({
  fork,
  finalizedCount,
}: {
  fork: ConversationForkFormContext;
  finalizedCount: number;
}) {
  const { t } = useTranslation();
  const estimate = fork.snapshot.descriptor.estimate;

  return (
    <>
      <div
        className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground"
        data-testid="conversation-fork-estimate"
      >
        {t("task:conversationForkEstimate", {
          tokens: new Intl.NumberFormat().format(estimate.estimated_tokens),
        })}
        {estimate.model_id && (
          <span className="ml-1">
            {t("task:conversationForkForModel", { model: estimate.model_id })}
          </span>
        )}
        <p>{t("task:conversationForkEstimateMethod", { method: estimate.method })}</p>
        {typeof estimate.context_limit === "number" && estimate.context_limit > 0 && (
          <p>
            {t("task:conversationForkEstimatePercent", {
              percent: Math.round((estimate.estimated_tokens / estimate.context_limit) * 100),
            })}
          </p>
        )}
        {estimate.attachments_unmeasured && (
          <p>{t("task:conversationForkAttachmentsUnmeasured")}</p>
        )}
        <p>{t("task:conversationForkEstimateScope")}</p>
      </div>
      <div className="flex items-center gap-2 text-sm font-medium">
        <IconEye className="size-4" aria-hidden="true" />
        {t("task:conversationForkHistoricalContent", {
          count: fork.snapshot.descriptor.message_count,
        })}
      </div>
      <pre
        className="min-h-32 whitespace-pre-wrap break-words rounded-md border bg-background p-3 text-xs leading-relaxed"
        data-testid="conversation-fork-content"
      >
        {fork.snapshot.content.content}
      </pre>
      {Object.keys(fork.snapshot.descriptor.omissions).length > 0 && (
        <details className="rounded-md border p-3">
          <summary className="min-h-7 cursor-pointer py-1 text-sm font-medium [@media(pointer:coarse)]:min-h-11">
            {t("task:conversationForkOmissions")}
          </summary>
          <ul className="grid gap-1 text-xs text-muted-foreground">
            {Object.entries(fork.snapshot.descriptor.omissions).map(([kind, count]) => (
              <li key={kind}>{t("task:conversationForkOmissionCount", { kind, count })}</li>
            ))}
          </ul>
        </details>
      )}
      <p className="text-xs text-muted-foreground">
        {t("task:conversationForkRangeBoundaryCount", { count: finalizedCount })}
      </p>
    </>
  );
}

export function ConversationForkPreview({ fork, onBack }: ConversationForkPreviewProps) {
  const { t } = useTranslation();
  const backButtonRef = useRef<HTMLButtonElement>(null);
  const [selection, setSelection] = useState(fork.selection);
  const [isApplying, setIsApplying] = useState(false);
  const finalized = useMemo(
    () => fork.source.boundaries.filter((boundary) => boundary.finalized),
    [fork.source.boundaries],
  );
  const unavailable = fork.source.attachments.filter((attachment) => !attachment.available);

  useEffect(() => {
    backButtonRef.current?.focus();
  }, []);

  const apply = async () => {
    setIsApplying(true);
    try {
      if (await fork.onApplySelection(selection)) onBack();
    } finally {
      setIsApplying(false);
    }
  };

  return (
    <section
      aria-label={t("task:conversationForkPreview")}
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="conversation-fork-preview"
    >
      <header className="flex shrink-0 items-center gap-2 border-b px-4 py-3">
        <Button
          ref={backButtonRef}
          type="button"
          variant="ghost"
          size="icon"
          onClick={onBack}
          aria-label={t("common:back")}
          className="h-7 min-h-7 w-7 min-w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:min-w-11"
        >
          <IconArrowLeft className="size-4" aria-hidden="true" />
        </Button>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">{t("task:conversationForkPreview")}</h2>
          <p className="truncate text-xs text-muted-foreground">{fork.source.title}</p>
        </div>
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
      </header>

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4">
        {fork.snapshotError?.expired && (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-destructive/40 p-3 text-sm"
          >
            <span>{t("task:conversationForkExpired")}</span>
            <Button
              type="button"
              variant="outline"
              onClick={() => void fork.onApplySelection(selection, true)}
              className="h-7 min-h-7 [@media(pointer:coarse)]:min-h-11"
            >
              {t("task:conversationForkRebuild")}
            </Button>
          </div>
        )}

        <ForkPreviewSelection
          fork={fork}
          selection={selection}
          onSelectionChange={setSelection}
          unavailableCount={unavailable.length}
        />
        <ForkPreviewSnapshot fork={fork} finalizedCount={finalized.length} />
      </div>

      <footer className="shrink-0 border-t bg-background px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))]">
        <Button
          type="button"
          onClick={() => void apply()}
          disabled={isApplying}
          className="h-7 min-h-7 w-full [@media(pointer:coarse)]:min-h-11"
        >
          {t("task:conversationForkApplySelection")}
        </Button>
      </footer>
    </section>
  );
}
