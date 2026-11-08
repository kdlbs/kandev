import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import type { ActionMeta } from "./action-message-details";
import { ActionButtons } from "./action-message-actions";
import { parseRetryAt, retryCountdownLabel } from "./transient-retry";
import { formatDateTime } from "@/lib/i18n/formats";
import { continuationActivityConfirmed, continuationPhase } from "./interruption-recovery-feedback";

function transientRetryReasonKey(failureCode?: string) {
  switch (failureCode) {
    case "model_capacity":
      return "chat:transientRetryReasonModelCapacity";
    case "network_unavailable":
      return "chat:transientRetryReasonNetworkUnavailable";
    case "provider_overloaded":
      return "chat:transientRetryReasonProviderOverloaded";
    case "provider_unavailable":
      return "chat:transientRetryReasonProviderUnavailable";
    case "rate_limited":
      return "chat:transientRetryReasonRateLimited";
    case "agent_transport_lost":
      return "chat:transientRetryReasonAgentTransportLost";
    case "provider_resource_exhausted":
      return "chat:transientRetryReasonResourceExhausted";
    default:
      return "chat:transientRetryReasonGeneric";
  }
}

function transientRetryContext(
  t: ReturnType<typeof useTranslation>["t"],
  provider?: string,
  model?: string,
) {
  if (!provider && !model) return null;
  if (provider && model) return t("chat:transientRetryProviderModel", { provider, model });
  if (provider) return t("chat:transientRetryProvider", { provider });
  return t("chat:transientRetryModel", { model });
}

export function TransientRetryNotice({
  metadata,
  taskId,
}: {
  metadata: ActionMeta;
  taskId?: string;
}) {
  const { t } = useTranslation();
  const phase = continuationPhase(metadata);
  const resumed = continuationActivityConfirmed(metadata);
  const retryAt = parseRetryAt(metadata.retry_at);
  const windowEndsAt = parseRetryAt(metadata.retry_window_ends_at);
  const fallbackDeadline = useMemo(
    () => Date.now() + Math.max(0, metadata.retry_in_seconds ?? 0) * 1_000,
    [metadata.attempt, metadata.retry_in_seconds],
  );
  const deadline = retryAt ?? fallbackDeadline;
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setNow(Date.now());
    if (phase && phase !== "waiting") return;
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [retryAt, metadata.retry_in_seconds, phase]);

  const remaining = Math.max(0, deadline - now);
  const provider = metadata.provider_name?.trim();
  const model = metadata.model_id?.trim();
  const context = transientRetryContext(t, provider, model);
  const waitingForWindowEnd = windowEndsAt !== undefined && retryAt === windowEndsAt;
  const countdown = retryNoticeCountdown(t, phase, remaining, waitingForWindowEnd);

  return (
    <section
      data-testid="transient-retry-card"
      role={phase && !resumed ? undefined : "status"}
      aria-live={phase && !resumed ? undefined : "polite"}
      className={`flex w-full min-w-0 flex-col items-stretch gap-2 rounded-md border px-2 py-1.5 sm:flex-row sm:items-center sm:gap-3 sm:px-3 ${
        resumed ? "border-border bg-muted/40" : "border-amber-500/25 bg-amber-500/[0.06]"
      }`}
    >
      {phase && !resumed && (
        <span className="sr-only" role="status" aria-live="polite">
          {t(`chat:continuationPhase_${phase}`)}
        </span>
      )}
      <NoticeIcon resumed={resumed} />
      <div className="min-w-0 flex-1 text-xs wrap-anywhere">
        {resumed ? (
          <ResumedContinuationBody context={context} phase={phase} />
        ) : (
          <RetryWarningBody
            metadata={metadata}
            phase={phase}
            countdown={countdown}
            context={context}
            windowEndsAt={windowEndsAt}
          />
        )}
      </div>
      {metadata.actions && metadata.actions.length > 0 && (
        <ActionButtons
          actions={metadata.actions}
          taskId={taskId}
          labelOverride={t("common:cancel")}
        />
      )}
    </section>
  );
}

function NoticeIcon({ resumed }: { resumed: boolean }) {
  if (resumed) {
    return (
      <IconInfoCircle className="h-4 w-4 flex-shrink-0 text-muted-foreground" aria-hidden="true" />
    );
  }
  return <IconAlertTriangle className="h-4 w-4 flex-shrink-0 text-amber-500" aria-hidden="true" />;
}

function ResumedContinuationBody({
  context,
  phase,
}: {
  context: string | null;
  phase: ReturnType<typeof continuationPhase>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-foreground">
        <span>{t("chat:continuationActivityResumed")}</span>
      </div>
      {context && <div className="mt-0.5 wrap-anywhere text-muted-foreground">{context}</div>}
      {phase && (
        <div className="mt-0.5 text-muted-foreground">{t("chat:continuationHistoryPreserved")}</div>
      )}
    </>
  );
}

function RetryWarningBody({
  metadata,
  phase,
  countdown,
  context,
  windowEndsAt,
}: {
  metadata: ActionMeta;
  phase: ReturnType<typeof continuationPhase>;
  countdown: string;
  context: string | null;
  windowEndsAt: ReturnType<typeof parseRetryAt>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-amber-600 dark:text-amber-400">
        <span>{t(transientRetryReasonKey(metadata.failure_code))}</span>
        <span aria-label={t("chat:transientRetryCountdownLabel")}>{countdown}</span>
      </div>
      {context && <div className="mt-0.5 wrap-anywhere text-muted-foreground">{context}</div>}
      {phase && (
        <div className="mt-0.5 text-muted-foreground">{t("chat:continuationHistoryPreserved")}</div>
      )}
      <div className="mt-0.5 text-muted-foreground">
        {t("chat:transientRetryAttempt", {
          attempt: metadata.attempt ?? 1,
          maxAttempts: metadata.max_attempts ?? 1,
        })}
      </div>
      {windowEndsAt !== undefined && (
        <div
          className="text-xs text-muted-foreground wrap-anywhere"
          data-testid="retry-window-deadline"
        >
          {t("chat:capacityRetryWindowEndsAt", { time: formatDateTime(windowEndsAt) })}
        </div>
      )}
    </>
  );
}

function continuationCountdown(
  t: ReturnType<typeof useTranslation>["t"],
  phase: "waiting" | "reconnecting" | "continuing",
  remaining: number,
) {
  if (phase === "waiting" && remaining > 0)
    return t("chat:continuationIn", { countdown: retryCountdownLabel(remaining) });
  return t(`chat:continuationPhase_${phase}`);
}

function retryCountdown(t: ReturnType<typeof useTranslation>["t"], remaining: number) {
  if (remaining > 0)
    return t("chat:transientRetryIn", { countdown: retryCountdownLabel(remaining) });
  return t("chat:transientRetryNow");
}

function retryNoticeCountdown(
  t: ReturnType<typeof useTranslation>["t"],
  phase: "waiting" | "reconnecting" | "continuing" | undefined,
  remaining: number,
  waitingForWindowEnd: boolean,
) {
  if (waitingForWindowEnd)
    return t("chat:capacityRetryWindowEnding", { countdown: retryCountdownLabel(remaining) });
  if (phase) return continuationCountdown(t, phase, remaining);
  return retryCountdown(t, remaining);
}
