"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Separator } from "@kandev/ui/separator";
import { useAutomationMCPOptions } from "@/hooks/domains/settings/use-automation-mcp-options";
import type { AutomationPluginToolGroup } from "@/lib/types/automation";
import { canonicalMCPSelection, canonicalPluginTools, type FormState } from "./automation-payload";
import { normalizeRepositorySelections } from "./automation-repository-selection";
import {
  AutomationMCPAccess,
  effectiveMCPSelection,
  type MCPOptionsState,
} from "./automation-mcp-access";
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

/** Whether the MCP choices differ from the saved ones, ignoring order. */
export function isMCPSelectionDirty(form: FormState, savedForm: FormState): boolean {
  return (
    JSON.stringify(canonicalMCPSelection(form.mcpSelection)) !==
    JSON.stringify(canonicalMCPSelection(savedForm.mcpSelection))
  );
}

/** Whether the plugin tool grants differ from the saved ones, ignoring order. */
export function isPluginToolsDirty(form: FormState, savedForm: FormState): boolean {
  return (
    JSON.stringify(canonicalPluginTools(form.pluginTools)) !==
    JSON.stringify(canonicalPluginTools(savedForm.pluginTools))
  );
}

/** Opens once each time a problem appears, then follows the person's toggles. */
function useAttentionDisclosure(needsAttention: boolean) {
  const [open, setOpen] = useState(needsAttention);
  const [attention, setAttention] = useState(needsAttention);
  if (attention !== needsAttention) {
    setAttention(needsAttention);
    if (needsAttention) setOpen(true);
  }
  return [open, setOpen] as const;
}

function pluginToolsNeedReview(
  savedForm: FormState,
  pluginTools: AgentToolsDisclosureProps["pluginTools"],
): boolean {
  if (pluginTools.loading || pluginTools.error) return false;
  return staleSelectionNames(savedForm.pluginTools, pluginTools.groups).length > 0;
}

function toolsSummary({
  form,
  optionsState,
  showPluginTools,
  t,
  language,
}: {
  form: FormState;
  optionsState: MCPOptionsState;
  showPluginTools: boolean;
  t: TFunction;
  language: string;
}): string {
  if (optionsState.options) {
    return summarizeAgentTools({
      selection: effectiveMCPSelection(form.mcpSelection, optionsState.options, form.taskMode),
      options: optionsState.options,
      pluginTools: showPluginTools ? form.pluginTools : [],
      translate: t,
      language,
    });
  }
  return t(optionsState.error ? "automations:toolsSummaryUnavailable" : "common:loading");
}

/**
 * MCP access and plugin tools behind one summary line. It opens by itself when
 * the MCP choices cannot load, saved MCP choices conflict, or approved plugin
 * tools need review.
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
  const optionsState = useAutomationMCPOptions(
    workspaceId,
    form.agentProfileId,
    selectedRepositoryIds(form, supportsMultiRepo),
  );
  const showPluginTools = form.taskMode === "automation_run";
  const [open, setOpen] = useAttentionDisclosure(
    optionsState.error ||
      mcp.saveFailed ||
      (showPluginTools && pluginToolsNeedReview(savedForm, pluginTools)),
  );
  const summary = toolsSummary({ form, optionsState, showPluginTools, t, language: i18n.language });
  return (
    <Collapsible
      open={open}
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
          {t(open ? "automations:toolsDone" : "automations:toolsEdit")}
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
          isDirty={isMCPSelectionDirty(form, savedForm)}
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
              isDirty={isPluginToolsDirty(form, savedForm)}
            />
          </>
        ) : null}
      </CollapsibleContent>
    </Collapsible>
  );
}
