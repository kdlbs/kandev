"use client";

import { useCallback, useState } from "react";
import {
  IconAlertTriangle,
  IconCheck,
  IconClipboard,
  IconChevronDown,
  IconLoader2,
  IconDownload,
  IconExternalLink,
  IconLock,
} from "@tabler/icons-react";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@kandev/ui/collapsible";
import { AgentLogo } from "@/components/agent-logo";
import { ProfileFormFields, type ProfileFormData } from "@/components/settings/profile-form-fields";
import type { AvailableAgent, ToolStatus } from "@/lib/types/http";

export type AgentSetting = {
  profileId: string;
  formData: ProfileFormData;
  dirty: boolean;
};

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [text]);

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="p-0.5 cursor-pointer hover:text-foreground text-muted-foreground transition-colors"
    >
      {copied ? (
        <IconCheck className="h-3 w-3 text-green-500" />
      ) : (
        <IconClipboard className="h-3 w-3" />
      )}
    </button>
  );
}

function StatusPill({ status }: { status: string }) {
  switch (status) {
    case "auth_required":
      return (
        <span className="flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
          <IconLock className="h-3.5 w-3.5" />
          No auth
        </span>
      );
    case "not_installed":
      return (
        <span className="flex items-center gap-1 text-xs text-muted-foreground">Not installed</span>
      );
    case "failed":
      return (
        <span className="flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
          <IconAlertTriangle className="h-3.5 w-3.5" />
          Error
        </span>
      );
    case "probing":
      return (
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          Probing
        </span>
      );
    case "not_configured":
      return <span className="flex items-center gap-1 text-xs text-muted-foreground">Pending</span>;
    default:
      return (
        <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
          <IconCheck className="h-3.5 w-3.5" />
          Installed
        </span>
      );
  }
}

function InstalledAgentRow({
  agent,
  settings,
  isOpen,
  onToggle,
  onUpdateSetting,
}: {
  agent: AvailableAgent;
  settings: AgentSetting | undefined;
  isOpen: boolean;
  onToggle: (open: boolean) => void;
  onUpdateSetting: (agentName: string, formPatch: Partial<ProfileFormData>) => void;
}) {
  const currentModel = settings?.formData.model || agent.model_config.default_model;
  const modelName =
    agent.model_config.available_models.find((m) => m.id === currentModel)?.name ?? currentModel;
  const status = agent.model_config.status ?? "ok";
  const showModelPill = status === "ok" && !!modelName;

  return (
    <Collapsible open={isOpen} onOpenChange={onToggle}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center gap-3 rounded-lg border p-2 text-left cursor-pointer hover:bg-muted/50 transition-colors group"
        >
          <AgentLogo agentName={agent.name} size={28} />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium">{agent.display_name}</p>
          </div>
          {showModelPill && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium truncate max-w-[120px]">
              {modelName}
            </span>
          )}
          <StatusPill status={status} />
          <IconChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border border-t-0 rounded-b-lg px-3 pb-3 pt-2">
          {settings && (
            <ProfileFormFields
              variant="compact"
              hideNameField
              hideCustomCLIFlags
              profile={settings.formData}
              onChange={(patch) => onUpdateSetting(agent.name, patch)}
              modelConfig={agent.model_config}
              permissionSettings={agent.permission_settings ?? {}}
              passthroughConfig={agent.passthrough_config ?? null}
              agentName={agent.name}
            />
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function NotInstalledItems({
  agents,
  tools,
  showLabel,
}: {
  agents: AvailableAgent[];
  tools: ToolStatus[];
  showLabel: boolean;
}) {
  if (agents.length === 0 && tools.length === 0) return null;
  return (
    <>
      {showLabel && <p className="text-xs text-muted-foreground mt-1">Not yet installed</p>}
      {agents.map((agent) => (
        <div
          key={agent.name}
          className="flex w-full items-center gap-3 rounded-lg border border-dashed p-2"
        >
          <AgentLogo agentName={agent.name} size={28} className="opacity-60" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-muted-foreground">{agent.display_name}</p>
            {agent.install_script && (
              <div className="flex items-center gap-1 mt-0.5">
                <code className="text-[10px] text-muted-foreground font-mono truncate">
                  {agent.install_script}
                </code>
                <CopyButton text={agent.install_script} />
              </div>
            )}
          </div>
          <IconDownload className="h-3.5 w-3.5 text-muted-foreground" />
        </div>
      ))}
      {tools.map((tool) => (
        <div
          key={tool.name}
          className="flex w-full items-center gap-3 rounded-lg border border-dashed p-2"
        >
          <IconDownload className="h-7 w-7 p-1 text-muted-foreground opacity-60 shrink-0" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-muted-foreground">{tool.display_name}</p>
            <p className="text-[10px] text-muted-foreground">{tool.description}</p>
            {tool.install_script && (
              <div className="flex items-center gap-1 mt-0.5">
                <code className="text-[10px] text-muted-foreground font-mono truncate">
                  {tool.install_script}
                </code>
                <CopyButton text={tool.install_script} />
              </div>
            )}
          </div>
          {tool.info_url && (
            <a
              href={tool.info_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground"
            >
              <IconExternalLink className="h-3.5 w-3.5" />
            </a>
          )}
        </div>
      ))}
    </>
  );
}

export function StepAgents({
  availableAgents,
  tools,
  agentSettings,
  loading,
  onUpdateSetting,
}: {
  availableAgents: AvailableAgent[];
  tools: ToolStatus[];
  agentSettings: Record<string, AgentSetting>;
  loading: boolean;
  onUpdateSetting: (agentName: string, formPatch: Partial<ProfileFormData>) => void;
}) {
  const [openAgent, setOpenAgent] = useState<string | null>(null);

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        Discovering agents...
      </div>
    );
  }

  const installedAgents = availableAgents.filter((a) => a.available);
  const notInstalledAgents = availableAgents.filter((a) => !a.available && a.install_script);
  const notInstalledTools = tools.filter((t) => !t.available);

  return (
    <div className="space-y-3">
      <div className="grid gap-2 max-h-[320px] overflow-y-auto pr-1">
        {installedAgents.map((agent) => (
          <InstalledAgentRow
            key={agent.name}
            agent={agent}
            settings={agentSettings[agent.name]}
            isOpen={openAgent === agent.name}
            onToggle={(isOpen) => setOpenAgent(isOpen ? agent.name : null)}
            onUpdateSetting={onUpdateSetting}
          />
        ))}
        <NotInstalledItems
          agents={notInstalledAgents}
          tools={notInstalledTools}
          showLabel={installedAgents.length > 0}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        Expand an agent to configure its model and permissions. Changes are saved when you proceed.
        You can also add custom TUI agents later in Settings &gt; Agents.
      </p>
      <p className="text-xs text-muted-foreground">
        <span className="text-yellow-500 font-medium">Careful:</span> The default Agent Profiles run
        with Auto Approve enabled (YOLO mode).
      </p>
    </div>
  );
}
