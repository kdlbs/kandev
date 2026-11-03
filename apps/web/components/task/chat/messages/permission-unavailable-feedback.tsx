"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { t } from "@/lib/i18n";

/**
 * Non-actionable feedback for an expired or unavailable permission decision.
 * Used by attached tool rows, whose standalone permission row is suppressed.
 */
export function PermissionUnavailableFeedback() {
  return (
    <div
      className="mt-1 flex items-center gap-1 text-xs text-muted-foreground"
      data-testid="permission-unavailable-feedback"
    >
      <IconAlertTriangle className="h-3 w-3 flex-shrink-0" aria-hidden="true" />
      <span>{t("task:permissionRequestNoLongerAvailable")}</span>
    </div>
  );
}
