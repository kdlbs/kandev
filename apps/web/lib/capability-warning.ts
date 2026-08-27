import { IconAlertTriangle, IconLock } from "@tabler/icons-react";
import type { CapabilityStatus } from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type CapabilityWarning = {
  Icon: typeof IconLock;
  color: string;
  title: string;
};

export function getCapabilityWarning(
  status: CapabilityStatus | undefined,
  error: string | undefined,
): CapabilityWarning | null {
  switch (status) {
    case "auth_required":
      return {
        Icon: IconLock,
        color: "text-amber-600 dark:text-amber-400",
        title: error || t("agents:authenticationRequired"),
      };
    case "not_installed":
      return {
        Icon: IconAlertTriangle,
        color: "text-muted-foreground",
        title: error || t("task:agentCliNotInstalled"),
      };
    case "failed":
      return {
        Icon: IconAlertTriangle,
        color: "text-red-600 dark:text-red-400",
        title: error || t("task:agentProbeFailed"),
      };
    default:
      return null;
  }
}
