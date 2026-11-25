"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AutomationMCPOptions,
  AutomationMCPSelection,
  TaskMode,
} from "@/lib/types/automation";
import { AutomationWorkspaceFields } from "./automation-mcp-access-fields";
import { AutomationBuiltinPackages } from "./automation-builtin-packages";

/** The MCP options request, owned by the caller so a summary can read it too. */
export type MCPOptionsState = {
  options?: AutomationMCPOptions;
  error: boolean;
  loading: boolean;
  reload: () => Promise<void>;
};

/** An unsaved selection starts from the normal-task defaults, or Core alone. */
export function effectiveMCPSelection(
  selection: AutomationMCPSelection | null | undefined,
  options: AutomationMCPOptions | undefined,
  taskMode: TaskMode,
): AutomationMCPSelection {
  if (selection) return selection;
  const defaults = taskMode === "normal_task" ? options?.task_defaults : undefined;
  return { builtin_packages: defaults ?? ["core"], definition_ids: [] };
}

/**
 * Present mode-specific MCP choices with touch controls and preserve selections during option
 * reload failures.
 */
export function AutomationMCPAccess({
  optionsState,
  taskMode,
  selection,
  onChange,
  isDirty,
  saveFailed,
  onReloadSaved,
}: {
  saveFailed: boolean;
  onReloadSaved: () => Promise<void>;
  optionsState: MCPOptionsState;
  taskMode: TaskMode;
  selection?: AutomationMCPSelection | null;
  onChange: (value: AutomationMCPSelection) => void;
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const { options, error, loading, reload } = optionsState;
  const value = effectiveMCPSelection(selection, options, taskMode);
  return (
    <section
      className="space-y-4"
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
      <AutomationWorkspaceFields
        options={options}
        value={value}
        onChange={onChange}
        touch={touch}
        disabled={loading || error}
      />
      <p className="text-xs text-muted-foreground">{t("automations:mcpThirdPartyPermissions")}</p>
    </section>
  );
}
