"use client";

import { useState } from "react";
import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import { MCPSelectionPicker } from "@/components/mcp/mcp-selection-picker";
import type { MCPInheritedSelection, MCPServerDefinition } from "@/lib/types/http-mcp";
import { cn } from "@/lib/utils";

type TaskCreateAdvancedSettingsProps = {
  isCreateMode: boolean;
  isEditMode?: boolean;
  isTaskStarted: boolean;
  blockedBy: string[];
  onBlockedByChange: (next: string[]) => void;
  dependenciesDisabled?: boolean;
  mcpDefinitions?: MCPServerDefinition[];
  mcpDefinitionsLoading?: boolean;
  mcpSelectionIds?: string[];
  onMcpSelectionIdsChange?: (ids: string[]) => void;
  mcpInheritedSelections?: MCPInheritedSelection[];
};

function TaskCreateMCPSettingRow({
  definitions,
  loading,
  selectedIds,
  onSelectedIdsChange,
  inherited,
  disabled,
}: {
  definitions: MCPServerDefinition[];
  loading: boolean;
  selectedIds: string[];
  onSelectedIdsChange: (ids: string[]) => void;
  inherited: MCPInheritedSelection[];
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 md:col-span-2" data-testid="task-create-mcp-setting-row">
      {loading ? (
        <p className="min-h-11 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
          {t("settings:mcpLoading")}
        </p>
      ) : (
        <MCPSelectionPicker
          definitions={definitions}
          selectedIds={selectedIds}
          onSelectedIdsChange={onSelectedIdsChange}
          inherited={inherited}
          disabled={disabled}
          label={t("settings:mcpServers")}
          description={t("settings:mcpSelectionDescription")}
          testId="task-create-mcp-selection"
        />
      )}
    </div>
  );
}

export function TaskCreateAdvancedSettings({
  isCreateMode,
  isEditMode = false,
  isTaskStarted,
  blockedBy,
  onBlockedByChange,
  dependenciesDisabled,
  mcpDefinitions = [],
  mcpDefinitionsLoading = false,
  mcpSelectionIds = [],
  onMcpSelectionIdsChange = () => undefined,
  mcpInheritedSelections = [],
}: TaskCreateAdvancedSettingsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if ((!isCreateMode && !isEditMode) || isTaskStarted) return null;

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
          <TaskCreateMCPSettingRow
            definitions={mcpDefinitions}
            loading={mcpDefinitionsLoading}
            selectedIds={mcpSelectionIds}
            onSelectedIdsChange={onMcpSelectionIdsChange}
            inherited={mcpInheritedSelections}
            disabled={dependenciesDisabled}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
