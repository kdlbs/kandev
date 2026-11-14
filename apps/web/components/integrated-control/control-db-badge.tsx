import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { useIntegratedControl } from "./integrated-control-provider";
import { toneSurfaceClass } from "./control-status-pill";

/**
 * The required-store reading in the control header: a quiet badge while the
 * store is healthy and the warning tone while it is unhealthy. The badge draws
 * nothing until the latest diagnostics read succeeded, and nothing while the
 * store is still initializing, so a failed or stale read never claims either
 * state; the page banner already reports a failed read.
 */
export function ControlDbBadge() {
  const { t } = useTranslation();
  const data = useIntegratedControl()?.data;
  const diagnostics = data?.persistence;
  if (!diagnostics || data?.persistenceStatus !== "ready") return null;
  if (diagnostics.state === "initializing") return null;
  const healthy = diagnostics.state === "healthy";
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded border px-1.5 py-0.5 text-[11px] font-medium",
        toneSurfaceClass(healthy ? "idle" : "delayed"),
      )}
      data-testid="control-db-badge"
      data-state={diagnostics.state}
    >
      {t(healthy ? "sidebar:controlSummary.dbHealthy" : "sidebar:controlSummary.dbUnhealthy")}
    </span>
  );
}
