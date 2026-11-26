"use client";

import { useId, useState, type ReactNode } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { TaskCreateRelationshipSettings } from "@/components/task-create-dialog-relationship-settings";
import { TaskCreateParentWorkspaceSetting } from "@/components/task-create-dialog-parent-workspace-setting";
import { TaskCreatePrioritySelect } from "@/components/task-create-dialog-priority-select";
import { Combobox } from "@/components/combobox";
import {
  TASK_CREATE_SETTING_LABEL,
  TASK_CREATE_SETTING_ROW,
  TASK_CREATE_SETTING_TRIGGER,
} from "@/components/task-create-dialog-setting-field";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  WorkflowAgentOverrideOption,
  WorkflowAgentOverrideRow,
} from "@/components/task-create-dialog-workflow-agent-overrides";
import { TaskCreateMCPSettingRow } from "@/components/task-create-dialog-mcp-select";
import type { MCPInheritedSelection, MCPServerDefinition } from "@/lib/types/http-mcp";
import { cn } from "@/lib/utils";
import type { TaskPriority } from "@/lib/types/http";
import type { TaskCreateParentWorkspaceSettingProps } from "@/components/task-create-dialog-parent-workspace-setting";

type TaskCreateAdvancedSettingsProps = TaskCreateParentWorkspaceSettingProps & {
  isCreateMode: boolean;
  isEditMode?: boolean;
  isTaskStarted: boolean;
  blockedBy: string[];
  onBlockedByChange: (next: string[]) => void;
  parentTaskId?: string;
  selectedParentTaskId?: string;
  onSelectedParentTaskIdChange?: (next: string | undefined) => void;
  relatedTaskIds?: string[];
  onRelatedTaskIdsChange?: (next: string[]) => void;
  contextStepId?: string | null;
  taskId?: string | null;
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
  builtinPackages?: ReactNode;
  mcpDefinitions?: MCPServerDefinition[];
  mcpDefinitionsLoading?: boolean;
  mcpSelectionIds?: string[];
  onMcpSelectionIdsChange?: (ids: string[]) => void;
  mcpInheritedSelections?: MCPInheritedSelection[];
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
  const selectorId = useId();
  const hintId = useId();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const heading = t("task:workflowAgents");
  const steps = row.stepNames.join(", ");
  const overridden = row.replacementProfileId !== "";
  const disabled = loading || dependenciesDisabled;
  const defaultLabel = t("task:workflowAgentDefaultSelection", { profile: row.sourceLabel });
  const restoreLabel = t("task:workflowAgentsUseDefault");
  const choices = [
    {
      value: row.sourceProfileId,
      label: defaultLabel,
      keywords: [restoreLabel, row.sourceLabel],
      renderLabel: () => <span className="truncate">{restoreLabel}</span>,
    },
    ...replacementOptionsForRow(row, options).filter(
      (option) => option.value !== row.sourceProfileId,
    ),
  ];
  return (
    <div
      className={cn(
        TASK_CREATE_SETTING_ROW,
        overridden &&
          "md:grid-cols-[var(--task-setting-label-width)_minmax(0,var(--task-setting-control-width))_auto]",
      )}
      data-testid={`task-create-workflow-agent-override-${row.sourceProfileId}`}
    >
      <label className={TASK_CREATE_SETTING_LABEL} htmlFor={selectorId}>
        {steps}
      </label>
      <div className="min-w-0">
        <Combobox
          options={choices}
          value={row.replacementProfileId || row.sourceProfileId}
          onValueChange={(value) =>
            onChange(row.sourceProfileId, value === row.sourceProfileId ? "" : value)
          }
          placeholder={defaultLabel}
          ariaLabel={steps}
          ariaDescribedBy={overridden ? hintId : undefined}
          dropdownLabel={touch ? undefined : heading}
          drawerTitle={touch ? steps : undefined}
          searchPlaceholder={t("task:searchAgents")}
          emptyMessage={t("task:noAgentFound")}
          disabled={disabled}
          triggerId={selectorId}
          triggerClassName={TASK_CREATE_SETTING_TRIGGER}
          testId="agent-profile-selector"
          plainTrigger
          touchTarget={touch}
          popoverPortal
        />
      </div>
      {overridden && (
        <div className="space-y-1 text-xs md:col-start-2 md:row-start-2" id={hintId}>
          <p className="text-muted-foreground">
            {t("task:workflowAgentDefaultHint", { profile: row.sourceLabel })}
          </p>
          {!row.replacementAvailable && (
            <p
              className="text-destructive"
              data-testid={`task-create-workflow-agent-override-error-${row.sourceProfileId}`}
            >
              {t("task:workflowAgentsUnavailable")}
            </p>
          )}
        </div>
      )}
      {overridden && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          className={controlSizingClassName("standard", "w-fit md:col-start-3 md:row-start-1")}
          onClick={() => onChange(row.sourceProfileId, "")}
          data-testid="task-create-workflow-agent-override-reset"
        >
          {t("task:workflowAgentRestoreDefault")}
        </Button>
      )}
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
  const showReset = error || rows.filter((row) => row.replacementProfileId !== "").length > 1;
  return (
    <section
      className="mt-4 min-w-0 border-t border-border pt-4"
      aria-invalid={invalid || undefined}
      data-testid="task-create-workflow-agent-overrides"
    >
      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
          <h3 className="text-sm font-semibold">{t("task:workflowAgents")}</h3>
          <span className="text-xs text-muted-foreground">{t("task:workflowAgentsTaskOnly")}</span>
        </div>
        {showReset && (
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
        )}
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

type OptionalRelationshipKeys =
  | "parentTaskId"
  | "selectedParentTaskId"
  | "onSelectedParentTaskIdChange"
  | "relatedTaskIds"
  | "onRelatedTaskIdsChange"
  | "contextStepId"
  | "taskId";

type NormalizedAdvancedSettingsProps = Required<
  Omit<TaskCreateAdvancedSettingsProps, OptionalRelationshipKeys>
> &
  Pick<TaskCreateAdvancedSettingsProps, OptionalRelationshipKeys>;

function valueOrDefault<T>(value: T | undefined, fallback: T): T {
  return value ?? fallback;
}

function normalizeAdvancedSettingsProps(
  props: TaskCreateAdvancedSettingsProps,
): NormalizedAdvancedSettingsProps {
  return {
    ...props,
    isEditMode: valueOrDefault(props.isEditMode, false),
    dependenciesDisabled: valueOrDefault(props.dependenciesDisabled, false),
    workflowAgentOverrideRows: valueOrDefault(props.workflowAgentOverrideRows, []),
    workflowAgentOverrideOptions: valueOrDefault(props.workflowAgentOverrideOptions, []),
    workflowAgentOverridesLoading: valueOrDefault(props.workflowAgentOverridesLoading, false),
    workflowAgentOverridesInvalid: valueOrDefault(props.workflowAgentOverridesInvalid, false),
    workflowAgentOverridesError: valueOrDefault(props.workflowAgentOverridesError, false),
    onWorkflowAgentOverrideChange: valueOrDefault(
      props.onWorkflowAgentOverrideChange,
      () => undefined,
    ),
    onResetWorkflowAgentOverrides: valueOrDefault(
      props.onResetWorkflowAgentOverrides,
      () => undefined,
    ),
    onRetryWorkflowAgentOverrides: valueOrDefault(
      props.onRetryWorkflowAgentOverrides,
      () => undefined,
    ),
    builtinPackages: props.builtinPackages ?? null,
    mcpDefinitions: valueOrDefault(props.mcpDefinitions, []),
    mcpDefinitionsLoading: valueOrDefault(props.mcpDefinitionsLoading, false),
    mcpSelectionIds: valueOrDefault(props.mcpSelectionIds, []),
    onMcpSelectionIdsChange: valueOrDefault(props.onMcpSelectionIdsChange, () => undefined),
    mcpInheritedSelections: valueOrDefault(props.mcpInheritedSelections, []),
  };
}

type AdvancedSettingsContentProps = Omit<
  NormalizedAdvancedSettingsProps,
  "isEditMode" | "isTaskStarted" | keyof TaskCreateParentWorkspaceSettingProps
> & { parentWorkspaceSetting: ReactNode };

function TaskCreateAdvancedSettingsContent({
  isCreateMode,
  blockedBy,
  onBlockedByChange,
  parentTaskId,
  selectedParentTaskId,
  onSelectedParentTaskIdChange,
  relatedTaskIds = [],
  onRelatedTaskIdsChange,
  contextStepId,
  taskId,
  priority,
  onPriorityChange,
  dependenciesDisabled,
  workflowAgentOverrideRows,
  workflowAgentOverrideOptions,
  workflowAgentOverridesLoading,
  workflowAgentOverridesInvalid,
  workflowAgentOverridesError,
  onWorkflowAgentOverrideChange,
  onResetWorkflowAgentOverrides,
  onRetryWorkflowAgentOverrides,
  builtinPackages,
  mcpDefinitions,
  mcpDefinitionsLoading,
  mcpSelectionIds,
  onMcpSelectionIdsChange,
  mcpInheritedSelections,
  parentWorkspaceSetting,
}: AdvancedSettingsContentProps) {
  const hasWorkflowAgentOverrides =
    workflowAgentOverridesLoading ||
    workflowAgentOverridesError ||
    workflowAgentOverrideRows.length > 0;
  return (
    <>
      <div className="grid min-w-0 gap-y-3" data-testid="task-create-advanced-settings-grid">
        <TaskCreateRelationshipSettings
          blockedBy={blockedBy}
          onBlockedByChange={onBlockedByChange}
          parentTaskId={parentTaskId}
          selectedParentTaskId={selectedParentTaskId}
          onSelectedParentTaskIdChange={onSelectedParentTaskIdChange}
          relatedTaskIds={relatedTaskIds}
          onRelatedTaskIdsChange={onRelatedTaskIdsChange}
          showCreateRelationships={isCreateMode}
          taskId={taskId}
          contextStepId={contextStepId}
          disabled={dependenciesDisabled}
        />
        <TaskCreatePrioritySelect value={priority} onChange={onPriorityChange} />
        <TaskCreateMCPSettingRow
          builtinPackages={builtinPackages}
          definitions={mcpDefinitions}
          loading={mcpDefinitionsLoading}
          selectedIds={mcpSelectionIds}
          onSelectedIdsChange={onMcpSelectionIdsChange}
          inherited={mcpInheritedSelections}
          disabled={dependenciesDisabled}
        />
        {parentWorkspaceSetting}
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

export function TaskCreateAdvancedSettings(props: TaskCreateAdvancedSettingsProps) {
  const settings = normalizeAdvancedSettingsProps(props);
  const {
    isCreateMode,
    isEditMode,
    isTaskStarted,
    initialWorkspaceLayout,
    onInitialWorkspaceLayoutChange,
    initialWorkspaceLayoutMode,
  } = settings;
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if ((!isCreateMode && !isEditMode) || isTaskStarted) return null;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="min-w-0 [--task-setting-label-width:9rem] [--task-setting-control-width:24rem]"
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
        className="min-w-0 pt-3"
        data-testid="task-create-advanced-settings-content"
      >
        <TaskCreateAdvancedSettingsContent
          {...settings}
          parentWorkspaceSetting={
            <TaskCreateParentWorkspaceSetting
              initialWorkspaceLayout={initialWorkspaceLayout}
              onInitialWorkspaceLayoutChange={onInitialWorkspaceLayoutChange}
              initialWorkspaceLayoutMode={initialWorkspaceLayoutMode}
              className="md:ml-[calc(var(--task-setting-label-width)+1rem)]"
            />
          }
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
