import { useTranslation } from "react-i18next";
import type { ControlRead } from "@/hooks/domains/coordinator/use-control-resource";
import { useIntegratedControl } from "./integrated-control-provider";
import { formatControlClock } from "./control-refresh-clock";

const STATUS_KEYS = {
  loading: "common:loading",
  denied: "sidebar:integratedControl.readDenied",
  unsupported: "sidebar:controlRestore.contractUnavailable",
  error: "sidebar:integratedControl.loadError",
};

export function ControlReadState({ status, receivedAt, stale }: ControlRead<unknown>) {
  const { t } = useTranslation();
  const bannerShown = Boolean(useIntegratedControl()?.failure);
  if (status === "ready" && !stale) return null;
  if (bannerShown && (status === "error" || stale)) return null;
  const retained = stale && receivedAt;
  const text = retained
    ? t("sidebar:controlRefresh.retainedAt", { time: formatControlClock(receivedAt) })
    : t(STATUS_KEYS[status === "ready" ? "error" : status]);
  return (
    <p
      role={status === "error" || status === "denied" ? "alert" : "status"}
      className="min-w-0 px-3 py-2 text-xs text-muted-foreground tabular-nums"
    >
      {text}
    </p>
  );
}
