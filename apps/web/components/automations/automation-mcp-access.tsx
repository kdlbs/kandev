"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAutomationMCPOptions } from "@/hooks/domains/settings/use-automation-mcp-options";
import type { AutomationMCPSelection, TaskMode } from "@/lib/types/automation";
import { AutomationWorkspaceFields } from "./automation-mcp-access-fields";
import { AutomationBuiltinPackages } from "./automation-builtin-packages";

/**
 * Present mode-specific MCP choices with touch controls and preserve selections during option
 * reload failures.
 */
export function AutomationMCPAccess({
  workspaceId,
  profileId,
  repositoryIds,
  taskMode,
  selection,
  onChange,
  isDirty,
  saveFailed,
  onReloadSaved,
}: {
  saveFailed: boolean;
  onReloadSaved: () => Promise<void>;
  workspaceId: string;
  profileId: string;
  repositoryIds: string[];
  taskMode: TaskMode;
  selection?: AutomationMCPSelection | null;
  onChange: (value: AutomationMCPSelection) => void;
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const { options, error, loading, reload } = useAutomationMCPOptions(
    workspaceId,
    profileId,
    repositoryIds,
  );
  const defaults = taskMode === "normal_task" ? options?.task_defaults : undefined;
  const value: AutomationMCPSelection = selection ?? {
    builtin_packages: defaults ?? ["core"],
    definition_ids: [],
  };
  return (
    <section
      className="space-y-4 rounded-lg border bg-card p-4"
      data-testid="automation-mcp-access"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
    >
      <h3 className="text-sm font-medium">{t("automations:mcpAccess")}</h3>
      <p className="text-xs text-muted-foreground">
        {t(taskMode === "normal_task" ? "automations:mcpNewTasks" : "automations:mcpNextStart")}
      </p>
      {error || saveFailed ? (
        <div role="alert" className="space-y-2 text-xs">
          <p>{t(saveFailed ? "settings:mcpSelectionConflict" : "automations:mcpOptionsFailed")}</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={touch ? "min-h-11" : "h-7"}
            onClick={() => void (saveFailed ? onReloadSaved() : reload())}
          >
            {t(saveFailed ? "settings:mcpReloadChoices" : "common:retry")}
          </Button>
        </div>
      ) : null}
      {loading ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t("common:loading")}
        </p>
      ) : null}
      <AutomationBuiltinPackages
        options={options}
        value={value}
        onChange={onChange}
        disabled={loading || error}
        taskMode={taskMode}
      />
      <p className="text-xs text-muted-foreground">{t("automations:mcpConfigurationAuthority")}</p>
      <AutomationWorkspaceFields
        options={options}
        value={value}
        onChange={onChange}
        touch={touch}
        disabled={loading || error}
      />
    </section>
  );
}
