import { useTranslation } from "react-i18next";
import { useIntegratedControl } from "./integrated-control-provider";
import { formatControlClock } from "./control-refresh-clock";

const FAILURE_KEYS = {
  unavailable: "sidebar:controlRefresh.bannerUnavailable",
  disconnected: "sidebar:controlRefresh.bannerDisconnected",
  error: "sidebar:controlRefresh.bannerError",
  denied: "sidebar:controlRefresh.bannerDenied",
} as const;

/** The header's read time: fixed-width digits, never replaced by a refreshing label. */
export function ControlLastRefresh() {
  const { t } = useTranslation();
  const receivedAt = useIntegratedControl()?.data?.receivedAt;
  if (!receivedAt) return null;
  return (
    <span
      className="shrink-0 whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground"
      data-testid="control-last-refresh"
    >
      {t("sidebar:controlRefresh.lastRefresh", { time: formatControlClock(receivedAt) })}
    </span>
  );
}

type Control = NonNullable<ReturnType<typeof useIntegratedControl>>;
type Translate = ReturnType<typeof useTranslation>["t"];

/** What the banner says, most important first; empty when nothing failed. */
function bannerParts(control: Control, t: Translate): string[] {
  const { failure, data, aggregate } = control;
  if (failure) {
    const parts = [t(FAILURE_KEYS[failure.kind])];
    if (data?.receivedAt)
      parts.push(
        t("sidebar:controlRefresh.bannerValuesAt", { time: formatControlClock(data.receivedAt) }),
      );
    if (failure.retryAt)
      parts.push(
        t("sidebar:controlRefresh.bannerRetryAt", { time: formatControlClock(failure.retryAt) }),
      );
    return parts;
  }
  if (aggregate.error && aggregate.data)
    return [t("sidebar:integratedControl.aggregate.staleError")];
  if (data && Object.values(data.byWorkspace).some((list) => list.error))
    return [t("sidebar:integratedControl.partialData")];
  return [];
}

/**
 * The one place a control screen reports a failed read: the workspace
 * snapshot, a workspace's coordinator list, or the Office summary. The last
 * values stay on screen underneath.
 */
export function ControlRefreshBanner() {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const parts = control ? bannerParts(control, t) : [];
  if (!control || parts.length === 0) return null;
  const [headline, ...details] = parts;
  // Only the headline is live: a new retry time is not re-announced every backoff step.
  return (
    <p
      className="shrink-0 border-b bg-amber-500/15 px-4 py-2 text-xs font-medium text-amber-800 tabular-nums dark:text-amber-200"
      data-testid="control-refresh-banner"
      data-kind={control.failure?.kind}
    >
      <span role="status">{headline}</span>
      {details.map((detail) => (
        <span key={detail}>
          {" · "}
          {detail}
        </span>
      ))}
    </p>
  );
}
