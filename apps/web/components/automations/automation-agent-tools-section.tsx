"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Separator } from "@kandev/ui/separator";
import { useAutomationMCPOptions } from "@/hooks/domains/settings/use-automation-mcp-options";
import type { AutomationPluginToolGroup } from "@/lib/types/automation";
import { canonicalMCPSelection, canonicalPluginTools, type FormState } from "./automation-payload";
import { normalizeRepositorySelections } from "./automation-repository-selection";
import { AutomationMCPAccess, effectiveMCPSelection } from "./automation-mcp-access";
import { AutomationPluginTools, staleSelectionNames } from "./automation-plugin-tools";
import { summarizeAgentTools } from "./agent-tools-summary";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

export type AgentToolsDisclosureProps = {
  form: FormState;
  savedForm: FormState;
  workspaceId: string;
  supportsMultiRepo: boolean;
  updateField: UpdateField;
  mcp: { saveFailed: boolean; reload: () => Promise<void> };
  pluginTools: {
    groups: AutomationPluginToolGroup[];
    loading: boolean;
    error: boolean;
    reload: () => void;
  };
};

function selectedRepositoryIds(form: FormState, supportsMultiRepo: boolean): string[] {
  if (form.repositoryMode !== "selected") return [];
  return normalizeRepositorySelections(form.repositorySelections, { supportsMultiRepo })
    .filter((item) => item.kind === "registered")
    .map((item) => item.id);
}

function pluginToolsNeedReview(
  savedForm: FormState,
  pluginTools: AgentToolsDisclosureProps["pluginTools"],
): boolean {
  if (pluginTools.loading || pluginTools.error) return false;
  return staleSelectionNames(savedForm.pluginTools, pluginTools.groups).length > 0;
}

/**
 * MCP access and plugin tools behind one summary line. It opens by itself when
 * saved MCP choices conflict or approved plugin tools need review.
 */
export function AgentToolsDisclosure({
  form,
  savedForm,
  workspaceId,
  supportsMultiRepo,
  updateField,
  mcp,
  pluginTools,
}: AgentToolsDisclosureProps) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const optionsState = useAutomationMCPOptions(
    workspaceId,
    form.agentProfileId,
    selectedRepositoryIds(form, supportsMultiRepo),
  );
  const showPluginTools = form.taskMode === "automation_run";
  const needsAttention =
    mcp.saveFailed || (showPluginTools && pluginToolsNeedReview(savedForm, pluginTools));
  const expanded = open || needsAttention;
  const summary = optionsState.options
    ? summarizeAgentTools({
        selection: effectiveMCPSelection(form.mcpSelection, optionsState.options, form.taskMode),
        options: optionsState.options,
        pluginTools: showPluginTools ? form.pluginTools : [],
        translate: t,
        language: i18n.language,
      })
    : t("common:loading");
  return (
    <Collapsible
      open={expanded}
      onOpenChange={setOpen}
      className="space-y-4"
      data-testid="automation-agent-tools"
    >
      <CollapsibleTrigger
        className="group flex w-full cursor-pointer items-center gap-3 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        data-testid="automation-agent-tools-toggle"
      >
        <span className="min-w-0 flex-1">
          <span className="block text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t("automations:toolsTitle")}
          </span>
          <span className="block truncate text-sm" data-testid="automation-agent-tools-summary">
            {summary}
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          {t(expanded ? "automations:toolsDone" : "automations:toolsEdit")}
          <IconChevronDown className="size-4 transition-transform group-data-[state=open]:rotate-180" />
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4">
        <AutomationMCPAccess
          saveFailed={mcp.saveFailed}
          onReloadSaved={mcp.reload}
          optionsState={optionsState}
          taskMode={form.taskMode}
          selection={form.mcpSelection}
          onChange={(value) => updateField("mcpSelection", value)}
          isDirty={
            JSON.stringify(canonicalMCPSelection(form.mcpSelection)) !==
            JSON.stringify(canonicalMCPSelection(savedForm.mcpSelection))
          }
        />
        {showPluginTools ? (
          <>
            <Separator />
            <AutomationPluginTools
              groups={pluginTools.groups}
              loading={pluginTools.loading}
              error={pluginTools.error}
              savedSelection={savedForm.pluginTools}
              selection={form.pluginTools}
              onChange={(value) => updateField("pluginTools", value)}
              onRetry={pluginTools.reload}
              isDirty={
                JSON.stringify(canonicalPluginTools(form.pluginTools)) !==
                JSON.stringify(canonicalPluginTools(savedForm.pluginTools))
              }
            />
          </>
        ) : null}
      </CollapsibleContent>
    </Collapsible>
  );
}
