"use client";

import { useState } from "react";
import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import { TaskCreatePrioritySelect } from "@/components/task-create-dialog-priority-select";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { AgentLogo } from "@/components/agent-logo";
import type {
  WorkflowAgentOverrideOption,
  WorkflowAgentOverrideRow,
} from "@/components/task-create-dialog-workflow-agent-overrides";
import { cn } from "@/lib/utils";
import type { TaskPriority } from "@/lib/types/http";

type TaskCreateAdvancedSettingsProps = {
  isCreateMode: boolean;
  isTaskStarted: boolean;
  blockedBy: string[];
  onBlockedByChange: (next: string[]) => void;
  priority: TaskPriority;
  onPriorityChange: (next: TaskPriority) => void;
  dependenciesDisabled?: boolean;
  workflowAgentOverrideRows?: WorkflowAgentOverrideRow[];
  workflowAgentOverrideOptions?: WorkflowAgentOverrideOption[];
  workflowAgentOverridesLoading?: boolean;
  workflowAgentOverridesInvalid?: boolean;
  workflowAgentOverridesError?: boolean;
  onWorkflowAgentOverrideChange?: (sourceProfileId: string, replacementProfileId: string) => void;
  onResetWorkflowAgentOverrides?: () => void;
  onRetryWorkflowAgentOverrides?: () => void;
};

function replacementOptionsForRow(
  row: WorkflowAgentOverrideRow,
  options: WorkflowAgentOverrideOption[],
): WorkflowAgentOverrideOption[] {
  if (row.replacementAvailable || !row.replacementProfileId) return options;
  return [
    {
      value: row.replacementProfileId,
      label: row.replacementLabel ?? row.replacementProfileId,
      renderLabel: () => <span>{row.replacementLabel ?? row.replacementProfileId}</span>,
    },
    ...options,
  ];
}

type WorkflowAgentOverrideRowProps = {
  row: WorkflowAgentOverrideRow;
  options: WorkflowAgentOverrideOption[];
  loading: boolean;
  dependenciesDisabled?: boolean;
  onChange: (sourceProfileId: string, replacementProfileId: string) => void;
};

function TaskCreateWorkflowAgentOverrideRow({
  row,
  options,
  loading,
  dependenciesDisabled,
  onChange,
}: WorkflowAgentOverrideRowProps) {
  const { t } = useTranslation();
  return (
    <div
      className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] sm:items-center"
      data-testid={`task-create-workflow-agent-override-${row.sourceProfileId}`}
    >
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2 text-xs text-foreground">
          {row.sourceAgentName && (
            <AgentLogo agentName={row.sourceAgentName} size={14} className="shrink-0" />
          )}
          <span className="truncate">{row.sourceLabel}</span>
        </div>
        <p className="mt-1 truncate text-[11px] text-muted-foreground">
          {t("task:workflowAgentsAffectedSteps", {
            steps: row.stepNames.join(", "),
          })}
        </p>
      </div>
      <div className="min-w-0">
        <AgentSelector
          options={replacementOptionsForRow(row, options)}
          value={row.replacementProfileId}
          onValueChange={(value) => onChange(row.sourceProfileId, value)}
          placeholder={t("task:workflowAgentsUseDefault")}
          disabled={loading || (dependenciesDisabled ?? false)}
          triggerClassName="h-11 min-h-11 md:h-7 md:min-h-7"
          popoverPortal
        />
        {!row.replacementAvailable && (
          <p
            className="mt-1 text-[11px] text-destructive"
            data-testid={`task-create-workflow-agent-override-error-${row.sourceProfileId}`}
          >
            {t("task:workflowAgentsUnavailable")}
          </p>
        )}
      </div>
    </div>
  );
}

type WorkflowAgentOverridesSectionProps = {
  rows: WorkflowAgentOverrideRow[];
  options: WorkflowAgentOverrideOption[];
  loading: boolean;
  invalid: boolean;
  error: boolean;
  dependenciesDisabled?: boolean;
  onChange: (sourceProfileId: string, replacementProfileId: string) => void;
  onReset: () => void;
  onRetry: () => void;
};

function TaskCreateWorkflowAgentOverridesBody({
  rows,
  options,
  loading,
  error,
  dependenciesDisabled,
  onChange,
}: Pick<
  WorkflowAgentOverridesSectionProps,
  "rows" | "options" | "loading" | "error" | "dependenciesDisabled" | "onChange"
>) {
  const { t } = useTranslation();
  if (error) {
    return (
      <div
        className="mt-3 flex min-w-0 flex-wrap items-center gap-2 text-xs text-destructive"
        role="alert"
        data-testid="task-create-workflow-agent-overrides-error"
      >
        <span>{t("task:workflowAgentsLoadError")}</span>
      </div>
    );
  }
  if (loading) {
    return (
      <p
        className="mt-3 text-xs text-muted-foreground"
        data-testid="task-create-workflow-agent-overrides-loading"
      >
        {t("task:workflowAgentsLoading")}
      </p>
    );
  }
  return (
    <div className="mt-3 min-w-0 space-y-3">
      {rows.map((row) => (
        <TaskCreateWorkflowAgentOverrideRow
          key={row.sourceProfileId}
          row={row}
          options={options}
          loading={loading}
          dependenciesDisabled={dependenciesDisabled}
          onChange={onChange}
        />
      ))}
    </div>
  );
}

function TaskCreateWorkflowAgentOverridesSection({
  rows,
  options,
  loading,
  invalid,
  error,
  dependenciesDisabled,
  onChange,
  onReset,
  onRetry,
}: WorkflowAgentOverridesSectionProps) {
  const { t } = useTranslation();
  return (
    <section
      className="mt-4 min-w-0 border-t border-border pt-4"
      aria-invalid={invalid || undefined}
      data-testid="task-create-workflow-agent-overrides"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-xs font-medium text-foreground">{t("task:workflowAgents")}</h3>
          <p className="mt-1 text-[11px] text-muted-foreground">
            {t("task:workflowAgentsTaskOnly")}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          className="min-h-11 shrink-0 px-2 text-[11px] md:min-h-7"
          disabled={loading}
          onClick={onReset}
          data-testid="task-create-workflow-agent-overrides-reset"
        >
          {t("task:workflowAgentsReset")}
        </Button>
      </div>
      <TaskCreateWorkflowAgentOverridesBody
        rows={rows}
        options={options}
        loading={loading}
        error={error}
        dependenciesDisabled={dependenciesDisabled}
        onChange={onChange}
      />
      {error && (
        <Button
          type="button"
          variant="ghost"
          className="min-h-11 px-2 text-[11px] md:min-h-7"
          onClick={onRetry}
          data-testid="task-create-workflow-agent-overrides-retry"
        >
          {t("task:workflowAgentsRetry")}
        </Button>
      )}
    </section>
  );
}

type AdvancedSettingsContentProps = Omit<
  TaskCreateAdvancedSettingsProps,
  "isCreateMode" | "isTaskStarted"
>;

function TaskCreateAdvancedSettingsContent({
  blockedBy,
  onBlockedByChange,
  priority,
  onPriorityChange,
  dependenciesDisabled,
  workflowAgentOverrideRows = [],
  workflowAgentOverrideOptions = [],
  workflowAgentOverridesLoading = false,
  workflowAgentOverridesInvalid = false,
  workflowAgentOverridesError = false,
  onWorkflowAgentOverrideChange = () => undefined,
  onResetWorkflowAgentOverrides = () => undefined,
  onRetryWorkflowAgentOverrides = () => undefined,
}: AdvancedSettingsContentProps) {
  const { t } = useTranslation();
  const hasWorkflowAgentOverrides =
    workflowAgentOverridesLoading ||
    workflowAgentOverridesError ||
    workflowAgentOverrideRows.length > 0;
  return (
    <>
      <div
        className="grid min-w-0 grid-cols-1 gap-4 px-1 md:grid-cols-2"
        data-testid="task-create-advanced-settings-grid"
      >
        <div
          className="flex min-w-0 items-center gap-3"
          data-testid="task-create-dependency-setting-row"
        >
          <div
            className="flex min-h-11 shrink-0 items-center gap-1 text-[11px] text-muted-foreground/70 md:min-h-6"
            data-testid="task-create-dependency-setting-label"
          >
            <span>{t("task:dependsOn")}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-11 min-h-11 w-11 min-w-11 cursor-pointer p-0 text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground md:h-6 md:min-h-6 md:w-6 md:min-w-6"
                  aria-label={t("task:dependencyInfoLabel")}
                  data-testid="task-create-dependency-setting-info"
                >
                  <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top" className="z-[60] max-w-xs">
                {t("task:dependencyInfo")}
              </TooltipContent>
            </Tooltip>
          </div>
          <div className="min-w-0 flex-1" data-testid="task-create-dependency-selector-container">
            <TaskCreateDependencies
              value={blockedBy}
              onChange={onBlockedByChange}
              disabled={dependenciesDisabled}
            />
          </div>
        </div>
        <div
          className="md:col-start-2 md:justify-self-start"
          data-testid="task-create-priority-setting-row"
        >
          <TaskCreatePrioritySelect value={priority} onChange={onPriorityChange} />
        </div>
      </div>
      {hasWorkflowAgentOverrides && (
        <TaskCreateWorkflowAgentOverridesSection
          rows={workflowAgentOverrideRows}
          options={workflowAgentOverrideOptions}
          loading={workflowAgentOverridesLoading}
          invalid={workflowAgentOverridesInvalid}
          error={workflowAgentOverridesError}
          dependenciesDisabled={dependenciesDisabled}
          onChange={onWorkflowAgentOverrideChange}
          onReset={onResetWorkflowAgentOverrides}
          onRetry={onRetryWorkflowAgentOverrides}
        />
      )}
    </>
  );
}

export function TaskCreateAdvancedSettings({
  isCreateMode,
  isTaskStarted,
  blockedBy,
  onBlockedByChange,
  priority,
  onPriorityChange,
  dependenciesDisabled,
  workflowAgentOverrideRows = [],
  workflowAgentOverrideOptions = [],
  workflowAgentOverridesLoading = false,
  workflowAgentOverridesInvalid = false,
  workflowAgentOverridesError = false,
  onWorkflowAgentOverrideChange = () => undefined,
  onResetWorkflowAgentOverrides = () => undefined,
  onRetryWorkflowAgentOverrides = () => undefined,
}: TaskCreateAdvancedSettingsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if (!isCreateMode || isTaskStarted) return null;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="min-w-0"
      data-testid="task-create-advanced-settings"
    >
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="min-h-12 h-12 w-full justify-start gap-1 px-1 text-[11px] text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground cursor-pointer md:h-7 md:min-h-7"
          data-testid="task-create-advanced-settings-trigger"
        >
          <span>{t("task:advancedSettings")}</span>
          <IconChevronDown
            className={cn("h-3 w-3 transition-transform", open && "rotate-180")}
            aria-hidden="true"
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent
        className="min-w-0 pt-1"
        data-testid="task-create-advanced-settings-content"
      >
        <TaskCreateAdvancedSettingsContent
          blockedBy={blockedBy}
          onBlockedByChange={onBlockedByChange}
          priority={priority}
          onPriorityChange={onPriorityChange}
          dependenciesDisabled={dependenciesDisabled}
          workflowAgentOverrideRows={workflowAgentOverrideRows}
          workflowAgentOverrideOptions={workflowAgentOverrideOptions}
          workflowAgentOverridesLoading={workflowAgentOverridesLoading}
          workflowAgentOverridesInvalid={workflowAgentOverridesInvalid}
          workflowAgentOverridesError={workflowAgentOverridesError}
          onWorkflowAgentOverrideChange={onWorkflowAgentOverrideChange}
          onResetWorkflowAgentOverrides={onResetWorkflowAgentOverrides}
          onRetryWorkflowAgentOverrides={onRetryWorkflowAgentOverrides}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
