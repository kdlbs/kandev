"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { ConfigureSessionOperation } from "@/lib/types/workflow-actions";
import type { SessionConfigCarryWarning } from "@/lib/workflows/session-config-carry-analysis";

export function SessionConfigCarryWarningPanel({
  warnings,
  onChoose,
  readOnly,
  disabled = false,
}: {
  warnings: SessionConfigCarryWarning[];
  onChoose: (warning: SessionConfigCarryWarning, operation: ConfigureSessionOperation) => void;
  readOnly: boolean;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3"
      data-testid="session-config-carry-warning"
    >
      <div className="flex items-start gap-2 text-xs text-amber-800 dark:text-amber-100">
        <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <span>{t("settings:sessionConfigCarryForwardWarning")}</span>
      </div>
      <div className="space-y-2">
        {warnings.map((warning) => (
          <div
            key={`${warning.agentName}-${warning.sourceStepId}`}
            className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between"
          >
            <span className="min-w-0 text-xs text-muted-foreground">{warning.message}</span>
            <div className="flex shrink-0 flex-wrap gap-1.5">
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-10 cursor-pointer"
                disabled={readOnly || disabled}
                onClick={() => onChoose(warning, "keep")}
              >
                {t("settings:sessionConfigKeep")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-10 cursor-pointer"
                disabled={readOnly || disabled}
                onClick={() => onChoose(warning, "restore_original")}
              >
                {t("settings:sessionConfigRestore")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-10 cursor-pointer"
                disabled={readOnly || disabled}
                onClick={() => onChoose(warning, "set")}
              >
                {t("settings:sessionConfigSetNew")}
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
