"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useSettingsSaveCoordinator } from "./settings-save-provider";
import type { RuntimePolicyController } from "./agent-runtime-policies";

/**
 * Explicit dirty-policy controls inside the manager's fixed footer. The global
 * floating save control stays outside the modal focus boundary, so this row is
 * the reachable Save/Reset path while the manager is open.
 */
export function RuntimePolicyDirtyFooter({
  controllers,
  canManage,
  embedded,
}: {
  controllers: RuntimePolicyController[];
  canManage: boolean;
  embedded?: boolean;
}) {
  const { t } = useTranslation();
  const coordinator = useSettingsSaveCoordinator();
  const dirty = controllers.filter((controller) => controller.isDirty);
  if (dirty.length === 0) return null;
  const failed = coordinator.status === "error" && coordinator.errorKind === "save";
  return (
    <div
      className={
        embedded ? "w-full space-y-2 border-b pb-2" : "w-full space-y-2 border-b px-4 pt-3 pb-2"
      }
      role="group"
      aria-label={t("agents:runtimePolicyDirtyLabel")}
      data-testid="runtime-policy-dirty-footer"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium">{t("agents:runtimePolicyDirtyTitle")}</p>
          <p className="text-xs text-muted-foreground">{t("agents:runtimePolicyDirtyHelp")}</p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            disabled={!canManage || coordinator.exclusiveBusy}
            onClick={() => dirty.forEach((controller) => controller.reset())}
            data-testid="runtime-policy-reset"
          >
            {t("common:reset")}
          </Button>
          <Button
            type="button"
            size="sm"
            className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            disabled={!canManage || coordinator.exclusiveBusy}
            onClick={() => void coordinator.saveAll()}
            data-testid="runtime-policy-save"
          >
            {t("agents:runtimePolicySaveChanges")}
          </Button>
        </div>
      </div>
      {failed && (
        <p className="flex items-start gap-1.5 text-xs text-destructive" role="alert">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          {t("agents:runtimePolicySaveFailed")}
        </p>
      )}
    </div>
  );
}
