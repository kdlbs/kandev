import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { SessionCapacityObservation } from "@/lib/types/system";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlReadState } from "./control-read-state";

export function ControlCapacityReadState({
  lastConfirmed,
}: {
  lastConfirmed?: SessionCapacityObservation;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const failed = control?.error || control?.data?.capacityStatus === "error";
  return (
    <>
      <ControlReadState
        status={control?.data?.capacityStatus ?? "ready"}
        refreshing={control?.loading}
        stale={failed}
        receivedAt={control?.data?.capacityReceivedAt}
      />
      <p className="text-xs text-muted-foreground">{t("sidebar:controlRefresh.capacityClock")}</p>
      <p
        className="min-h-12 text-xs text-muted-foreground md:min-h-8"
        data-testid="control-capacity-last-confirmed"
      >
        {t("sidebar:controlRefresh.lastConfirmed")}:{" "}
        {lastConfirmed ? (
          <>
            {lastConfirmed.in_use} / {lastConfirmed.limit} ·{" "}
            <time dateTime={lastConfirmed.observed_at}>
              {formatDateTime(lastConfirmed.observed_at)}
            </time>{" "}
            ({Intl.DateTimeFormat().resolvedOptions().timeZone})
          </>
        ) : (
          t("common:unknown")
        )}
      </p>
    </>
  );
}
