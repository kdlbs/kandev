"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChartBar, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { DrawerClose } from "@kandev/ui/drawer";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useConversationUsage } from "@/hooks/domains/session/use-conversation-usage";
import type { UsageResponse, UsageTokenBreakdown, UsageTurn } from "@/lib/types/conversation-usage";
import {
  formatUsageCost,
  formatUsageTokens,
  usageDetailForTurn,
} from "@/lib/utils/conversation-usage";

type ConversationUsageDisplayProps = {
  taskId?: string | null;
  sessionId: string | null;
};

function UsageValue({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-sm">
      <span className="min-w-0 truncate text-muted-foreground">{label}</span>
      <span className="shrink-0 font-medium tabular-nums">{formatUsageTokens(value)}</span>
    </div>
  );
}

function UsageResponseDetails({ response }: { response?: UsageResponse }) {
  const { t } = useTranslation();
  if (!response) {
    return (
      <p className="text-sm text-muted-foreground">{t("task:conversationUsage.notAvailable")}</p>
    );
  }
  return (
    <div className="space-y-1" data-testid="usage-last-response">
      <UsageValue label={t("task:conversationUsage.input")} value={response.input_tokens} />
      {response.cached_read_tokens !== undefined && (
        <UsageValue
          label={t("task:conversationUsage.cachedRead")}
          value={response.cached_read_tokens}
        />
      )}
      {response.reported_cache_write_tokens !== undefined && (
        <UsageValue
          label={t("task:conversationUsage.cachedWrite")}
          value={response.reported_cache_write_tokens}
        />
      )}
      {response.output_tokens !== undefined && (
        <UsageValue label={t("task:conversationUsage.output")} value={response.output_tokens} />
      )}
      {response.reasoning_output_tokens !== undefined && (
        <UsageValue
          label={t("task:conversationUsage.reasoning")}
          value={response.reasoning_output_tokens}
        />
      )}
    </div>
  );
}

function UsageBreakdown({ label, usage }: { label: string; usage: UsageTokenBreakdown }) {
  const { t } = useTranslation();
  return (
    <section className="space-y-1">
      <h3 className="text-xs font-semibold text-muted-foreground">{label}</h3>
      <UsageValue label={t("task:conversationUsage.input")} value={usage.input_tokens} />
      <UsageValue label={t("task:conversationUsage.cachedRead")} value={usage.cached_read_tokens} />
      {usage.cached_write_tokens > 0 && (
        <UsageValue
          label={t("task:conversationUsage.cachedWrite")}
          value={usage.cached_write_tokens}
        />
      )}
      <UsageValue label={t("task:conversationUsage.output")} value={usage.output_tokens} />
      {usage.thought_tokens > 0 && (
        <UsageValue label={t("task:conversationUsage.reasoning")} value={usage.thought_tokens} />
      )}
      <UsageValue label={t("task:conversationUsage.totalTokens")} value={usage.total_tokens} />
      {usage.overflow && (
        <p className="text-xs text-muted-foreground">{t("task:conversationUsage.partial")}</p>
      )}
    </section>
  );
}

type ConversationUsage = ReturnType<typeof useConversationUsage>;

function useCostUnavailableLabel() {
  const { t } = useTranslation();
  return t("task:conversationUsage.costUnavailable");
}

function UsageLoadStatus({ usage }: { usage: ConversationUsage }) {
  const { t } = useTranslation();
  return (
    <>
      {usage.loading && !usage.totals && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("task:conversationUsage.pending")}
        </p>
      )}
      {usage.error && (
        <div role="alert" className="flex items-center justify-between gap-3">
          <p className="text-sm text-destructive">{t("task:conversationUsage.loadFailed")}</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="min-h-11"
            onClick={() => void usage.refresh()}
          >
            {t("task:conversationUsage.retry")}
          </Button>
        </div>
      )}
      {usage.detail?.error && (
        <div role="alert" className="flex items-center justify-between gap-3">
          <p className="text-sm text-destructive">{t("task:conversationUsage.loadFailed")}</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="min-h-11"
            onClick={() => usage.latestTurn && void usage.loadTurnDetail(usage.latestTurn.turn_id)}
          >
            {t("task:conversationUsage.retry")}
          </Button>
        </div>
      )}
    </>
  );
}

function UsageTurnSummary({ turn }: { turn: UsageTurn | null }) {
  const { t } = useTranslation();
  const costUnavailable = useCostUnavailableLabel();
  if (!turn) {
    return (
      <section className="space-y-1" data-testid="usage-turn-summary">
        <h3 className="text-xs font-semibold text-muted-foreground">
          {t("task:conversationUsage.thisTurn")}
        </h3>
        <p className="text-sm text-muted-foreground">{t("task:conversationUsage.notAvailable")}</p>
      </section>
    );
  }
  const turnCost = formatUsageCost(turn.total.cost_subcents);
  const estimated = turn.cost_sources.includes("models_dev_list");
  const costLabel = !turnCost
    ? costUnavailable
    : t(estimated ? "task:conversationUsage.estimatedCost" : "task:conversationUsage.recordedCost");
  return (
    <section className="space-y-1" data-testid="usage-turn-summary">
      <h3 className="text-xs font-semibold text-muted-foreground">
        {t("task:conversationUsage.thisTurn")}
      </h3>
      <UsageValue label={t("task:conversationUsage.totalTokens")} value={turn.total.total_tokens} />
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span className="text-muted-foreground">{costLabel}</span>
        <span className="font-medium tabular-nums">
          {turnCost ?? t("task:conversationUsage.notAvailable")}
        </span>
      </div>
      {turn.completeness !== "exact" && (
        <p className="text-xs text-muted-foreground">
          {t(
            turn.completeness === "estimated"
              ? "task:conversationUsage.estimated"
              : "task:conversationUsage.partial",
          )}
        </p>
      )}
    </section>
  );
}

function UsageCostSources({ sources }: { sources: string[] }) {
  const { t } = useTranslation();
  const costUnavailable = useCostUnavailableLabel();
  const labelForSource = (source: string) => {
    switch (source) {
      case "models_dev_list":
        return t("task:conversationUsage.estimatedCost");
      case "provider_reported":
        return t("task:conversationUsage.providerReportedCost");
      default:
        return costUnavailable;
    }
  };
  return (
    <section className="space-y-1">
      <h3 className="text-xs font-semibold text-muted-foreground">
        {t("task:conversationUsage.priceSource")}
      </h3>
      {sources.length > 0 ? (
        <p className="text-sm">{sources.map(labelForSource).join(", ")}</p>
      ) : (
        <p className="text-sm text-muted-foreground">{costUnavailable}</p>
      )}
    </section>
  );
}

function UsageTurnDetails({ usage, turn }: { usage: ConversationUsage; turn: UsageTurn }) {
  const { t } = useTranslation();
  const response = usage.detail?.turn?.last_response ?? turn.last_response;
  const responses = usage.detail?.turn?.responses ?? [];
  return (
    <>
      <section className="space-y-1">
        <h3 className="text-xs font-semibold text-muted-foreground">
          {t("task:conversationUsage.lastResponse")}
        </h3>
        <UsageResponseDetails response={response} />
      </section>
      {responses.length > 1 && (
        <section className="space-y-2 border-b pb-3">
          <h3 className="text-xs font-semibold text-muted-foreground">
            {t("task:conversationUsage.responseDetails")}
          </h3>
          {responses.map((item) => (
            <div key={item.usage_event_id} className="rounded-md border p-2">
              <UsageResponseDetails response={item} />
            </div>
          ))}
        </section>
      )}
      <div className="grid grid-cols-1 gap-3 border-y py-3 sm:grid-cols-2">
        <UsageBreakdown label={t("task:conversationUsage.direct")} usage={turn.direct} />
        <UsageBreakdown label={t("task:conversationUsage.child")} usage={turn.child} />
      </div>
      <UsageCostSources sources={turn.cost_sources} />
    </>
  );
}

function UsageSessionSummary({ usage }: { usage: ConversationUsage }) {
  const { t } = useTranslation();
  const costUnavailable = useCostUnavailableLabel();
  const sessionCost =
    usage.totals && usage.totals.unpriced_event_count === 0
      ? formatUsageCost(usage.totals.cost_subcents_decimal)
      : null;
  return (
    <section className="space-y-1 border-t pt-3">
      <h3 className="text-xs font-semibold text-muted-foreground">
        {t("task:conversationUsage.sessionTotal")}
      </h3>
      {usage.totals ? (
        <>
          <UsageValue
            label={t("task:conversationUsage.totalTokens")}
            value={usage.totals.tokens_total}
          />
          <div className="flex items-baseline justify-between gap-3 text-sm">
            <span className="text-muted-foreground">
              {t("task:conversationUsage.recordedCost")}
            </span>
            <span className="font-medium tabular-nums">{sessionCost ?? costUnavailable}</span>
          </div>
          {usage.totals.estimated_event_count > 0 && (
            <p className="text-xs text-muted-foreground">
              {t("task:conversationUsage.estimatedCount", {
                count: usage.totals.estimated_event_count,
              })}
            </p>
          )}
        </>
      ) : (
        <p className="text-sm text-muted-foreground">{t("task:conversationUsage.pending")}</p>
      )}
    </section>
  );
}

function UsageDisclosureActions({
  usage,
  turn,
  close,
}: {
  usage: ConversationUsage;
  turn: ConversationUsage["latestTurn"];
  close: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {turn?.turn_id && !usage.detail?.turn && (
        <Button
          type="button"
          variant="outline"
          className="min-h-11 w-full"
          disabled={usage.detail?.loading}
          onClick={() => void usage.loadTurnDetail(turn.turn_id)}
        >
          {t("task:conversationUsage.viewResponses")}
        </Button>
      )}
      {usage.detail?.loading && (
        <p role="status" className="text-xs text-muted-foreground">
          {t("task:conversationUsage.pending")}
        </p>
      )}
      {usage.detail?.turn?.responses && usage.detail.turn.responses.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("task:conversationUsage.responseCount", {
            count: usage.detail.turn.responses.length,
          })}
        </p>
      )}
      <Button type="button" variant="ghost" className="min-h-11 w-full" onClick={close}>
        {t("common:close")}
      </Button>
    </>
  );
}

function UsageDisclosure({ usage, close }: { usage: ConversationUsage; close: () => void }) {
  const detail = usageDetailForTurn(usage.latestTurn, usage.detail);
  const currentUsage = detail ? usage : { ...usage, detail: null };
  const turn = detail?.turn ?? usage.latestTurn;
  return (
    <div className="space-y-4" data-testid="conversation-usage-content">
      <UsageLoadStatus usage={currentUsage} />
      {turn ? (
        <>
          <UsageTurnSummary turn={turn} />
          <UsageTurnDetails usage={currentUsage} turn={turn} />
        </>
      ) : (
        <UsageTurnSummary turn={null} />
      )}
      <UsageSessionSummary usage={currentUsage} />
      <UsageDisclosureActions usage={currentUsage} turn={turn} close={close} />
    </div>
  );
}

export function ConversationUsageDisplay({ taskId, sessionId }: ConversationUsageDisplayProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const touch = useTouchDrawer();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const usage = useConversationUsage(taskId ?? null, sessionId);
  const hasUsage = (usage.totals?.event_count ?? 0) > 0;
  if (!taskId || !sessionId || (!hasUsage && !usage.loading && !usage.error)) return null;

  const openDisclosure = () => {
    setOpen(true);
    if (usage.latestTurn) void usage.loadTurnDetail(usage.latestTurn.turn_id);
  };
  const closeFocus = (event: Event) => {
    event.preventDefault();
    queueMicrotask(() => triggerRef.current?.focus());
  };
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant="ghost"
      className={touch ? "h-11 min-w-11 gap-1 px-2 text-xs" : "h-8 gap-1 px-2 text-xs"}
      aria-label={t("task:conversationUsage.open")}
      data-testid="conversation-usage-trigger"
      onClick={openDisclosure}
    >
      <IconChartBar className="h-4 w-4" aria-hidden="true" />
      <span>{t("task:conversationUsage.open")}</span>
    </Button>
  );

  if (touch) {
    return (
      <>
        {trigger}
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("task:conversationUsage.title")}
          contentTestId="conversation-usage-drawer"
          onCloseAutoFocus={closeFocus}
          headerAction={
            <DrawerClose asChild>
              <Button
                type="button"
                variant="ghost"
                className="h-11 min-w-11"
                aria-label={t("common:close")}
              >
                <IconX className="h-4 w-4" />
              </Button>
            </DrawerClose>
          }
        >
          <UsageDisclosure usage={usage} close={() => setOpen(false)} />
        </MobilePickerSheet>
      </>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="end"
        side="top"
        className="max-h-[min(75dvh,34rem)] w-[min(24rem,calc(100vw-2rem))] overflow-y-auto p-4"
        data-testid="conversation-usage-popover"
      >
        <div className="mb-3 flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">{t("task:conversationUsage.title")}</h2>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            aria-label={t("common:close")}
            onClick={() => setOpen(false)}
          >
            <IconX className="h-4 w-4" />
          </Button>
        </div>
        <UsageDisclosure usage={usage} close={() => setOpen(false)} />
      </PopoverContent>
    </Popover>
  );
}
