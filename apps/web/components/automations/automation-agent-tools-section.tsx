"use client";

import { useTranslation } from "react-i18next";
import { Separator } from "@kandev/ui/separator";
import type { AutomationPluginToolGroup } from "@/lib/types/automation";
import { canonicalMCPSelection, canonicalPluginTools, type FormState } from "./automation-payload";
import { normalizeRepositorySelections } from "./automation-repository-selection";
import { AutomationMCPAccess } from "./automation-mcp-access";
import { AutomationPluginTools } from "./automation-plugin-tools";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

type AgentToolsSectionProps = {
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

/** MCP access and plugin tools: what the agent can use during a run. */
export function AgentToolsSection({
  form,
  savedForm,
  workspaceId,
  supportsMultiRepo,
  updateField,
  mcp,
  pluginTools,
}: AgentToolsSectionProps) {
  const { t } = useTranslation();
  // A managed conversation runs in the plugin's own session, so neither applies.
  if (form.taskMode === "managed_conversation") return null;
  const showPluginTools = form.taskMode === "automation_run";
  const repositoryIds =
    form.repositoryMode === "selected"
      ? normalizeRepositorySelections(form.repositorySelections, { supportsMultiRepo })
          .filter((item) => item.kind === "registered")
          .map((item) => item.id)
      : [];
  return (
    <section className="space-y-2" data-testid="automation-agent-tools">
      <div>
        <h3 className="text-base font-medium">{t("automations:agentToolsTitle")}</h3>
        <p className="text-sm text-muted-foreground">{t("automations:agentToolsDescription")}</p>
      </div>
      <div className="space-y-4 rounded-lg border bg-card p-4">
        <AutomationMCPAccess
          saveFailed={mcp.saveFailed}
          onReloadSaved={mcp.reload}
          workspaceId={workspaceId}
          profileId={form.agentProfileId}
          repositoryIds={repositoryIds}
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
      </div>
    </section>
  );
}
