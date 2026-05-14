"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  IconAlertTriangle,
  IconCheck,
  IconClipboard,
  IconDownload,
  IconExternalLink,
  IconLoader2,
  IconPlus,
  IconRefresh,
  IconTerminal2,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import {
  createCustomTUIAgent,
  installAgent,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listInstallJobs,
} from "@/lib/api";
import type { InstallJob } from "@/lib/api";
import { useAgentDiscovery } from "@/hooks/domains/settings/use-agent-discovery";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AgentLogo } from "@/components/agent-logo";
import { AddTUIAgentDialog } from "@/components/settings/add-tui-agent-dialog";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import { InstallAgentCard } from "@/components/settings/install-agent-card";
import { InstalledAgentCard } from "@/components/settings/installed-agent-card";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type {
  AgentDiscovery,
  Agent,
  AvailableAgent,
  AgentProfile,
  ToolStatus,
} from "@/lib/types/http";

function useCopyCommand() {
  const [copiedValue, setCopiedValue] = useState<string | null>(null);
  const copy = useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // fallback
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    setCopiedValue(text);
    setTimeout(() => setCopiedValue(null), 2000);
  }, []);
  return { copiedValue, copy };
}

function CopyButton({
  text,
  copiedValue,
  onCopy,
}: {
  text: string;
  copiedValue: string | null;
  onCopy: (text: string) => void;
}) {
  const isCopied = copiedValue === text;
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-7 w-7 p-0 cursor-pointer shrink-0"
      aria-label={isCopied ? "Copied" : "Copy install command"}
      onClick={() => onCopy(text)}
    >
      {isCopied ? (
        <IconCheck className="h-3.5 w-3.5 text-green-500" />
      ) : (
        <IconClipboard className="h-3.5 w-3.5 text-muted-foreground" />
      )}
    </Button>
  );
}

function InstallCard({
  agent,
  copiedValue,
  onCopy,
  job,
  onInstall,
}: {
  agent: AvailableAgent;
  copiedValue: string | null;
  onCopy: (text: string) => void;
  job: InstallJob | undefined;
  onInstall: (name: string) => void;
}) {
  return (
    <InstallAgentCard
      agent={agent}
      job={job}
      onInstall={onInstall}
      scriptSlot={
        agent.install_script ? (
          <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
            <code className="flex-1 truncate">{agent.install_script}</code>
            <CopyButton text={agent.install_script} copiedValue={copiedValue} onCopy={onCopy} />
          </div>
        ) : null
      }
    />
  );
}

function ToolInstallCard({
  tool,
  copiedValue,
  onCopy,
}: {
  tool: ToolStatus;
  copiedValue: string | null;
  onCopy: (text: string) => void;
}) {
  return (
    <Card className="border-dashed">
      <CardContent className="py-4 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <IconDownload className="h-5 w-5 text-muted-foreground shrink-0" />
          <h4 className="font-medium">{tool.display_name}</h4>
          {tool.available && (
            <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
              <IconCheck className="h-3.5 w-3.5" />
              Installed
            </span>
          )}
        </div>
        {tool.description && <p className="text-xs text-muted-foreground">{tool.description}</p>}
        {!tool.available && tool.install_script && (
          <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
            <code className="flex-1 truncate">{tool.install_script}</code>
            <CopyButton text={tool.install_script} copiedValue={copiedValue} onCopy={onCopy} />
          </div>
        )}
        {tool.info_url && (
          <a
            href={tool.info_url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <IconExternalLink className="h-3 w-3" />
            {tool.info_url}
          </a>
        )}
      </CardContent>
    </Card>
  );
}

type ProfileListItemProps = {
  agent: Agent;
  profile: AgentProfile;
};

function ProfileListItem({ agent, profile }: ProfileListItemProps) {
  const profilePath = `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${profile.id}`;
  return (
    <Link href={profilePath} className="block">
      <Card className="hover:bg-accent transition-colors cursor-pointer">
        <CardContent className="py-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <AgentLogo agentName={agent.name} className="shrink-0" />
            <span className="text-sm font-medium">
              {agent.profiles[0]?.agent_display_name ?? agent.name}
            </span>
            {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
            <span className="text-sm text-muted-foreground">{profile.name}</span>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

type InstalledAgentsSectionProps = {
  installedAgents: AgentDiscovery[];
  discoveryLoading: boolean;
  rescanning: boolean;
  savedAgentsByName: Map<string, Agent>;
  resolveDisplayName: (name: string) => string;
  resolveCapabilityStatus: (name: string) => string | undefined;
  setTuiDialogOpen: (open: boolean) => void;
  handleRescan: () => Promise<void>;
};

function InstalledAgentsSection({
  installedAgents,
  discoveryLoading,
  rescanning,
  savedAgentsByName,
  resolveDisplayName,
  resolveCapabilityStatus,
  setTuiDialogOpen,
  handleRescan,
}: InstalledAgentsSectionProps) {
  const [shellOpen, setShellOpen] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold">Installed Agents</h3>
          <p className="text-sm text-muted-foreground">
            Agents detected on this machine are ready to configure.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShellOpen(true)}
            className="cursor-pointer"
            data-testid="open-host-shell"
          >
            <IconTerminal2 className="h-4 w-4 mr-2" />
            Terminal
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setTuiDialogOpen(true)}
            className="cursor-pointer"
          >
            <IconPlus className="h-4 w-4 mr-2" />
            Add TUI Agent
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleRescan}
            disabled={rescanning}
            className="cursor-pointer"
          >
            {rescanning ? (
              <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
            ) : (
              <IconRefresh className="h-4 w-4 mr-2" />
            )}
            Rescan
          </Button>
        </div>
      </div>
      <HostShellDialog
        open={shellOpen}
        onOpenChange={setShellOpen}
        onClose={() => {
          // Rescan after the user closes the shell - they may have installed
          // an agent CLI or fixed an auth issue inside the terminal.
          void handleRescan();
        }}
      />

      {installedAgents.length === 0 && (
        <Card>
          <CardContent className="py-8 text-center">
            <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
              {discoveryLoading ? (
                <span>Scanning for installed agents...</span>
              ) : (
                <>
                  <IconAlertTriangle className="h-4 w-4" />
                  No installed agents were detected. Install one below, then click Rescan.
                </>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
        {installedAgents.map((agent: AgentDiscovery) => (
          <InstalledAgentCard
            key={agent.name}
            agent={agent}
            savedAgent={savedAgentsByName.get(agent.name)}
            displayName={resolveDisplayName(agent.name)}
            capabilityStatus={resolveCapabilityStatus(agent.name)}
            onAuthComplete={() => void handleRescan()}
          />
        ))}
      </div>
    </div>
  );
}

function SuggestInstallSection({
  notInstalledAgents,
  tools,
  copiedValue,
  onCopy,
  installJobs,
  onInstall,
}: {
  notInstalledAgents: AvailableAgent[];
  tools: ToolStatus[];
  copiedValue: string | null;
  onCopy: (text: string) => void;
  installJobs: Record<string, InstallJob>;
  onInstall: (name: string) => void;
}) {
  const notInstalledTools = tools.filter((t) => !t.available);
  if (notInstalledAgents.length === 0 && notInstalledTools.length === 0) return null;

  return (
    <div className="space-y-4">
      <Separator />
      <div>
        <h3 className="text-lg font-semibold">Available to Install</h3>
        <p className="text-sm text-muted-foreground">
          Click Install to run the agent&apos;s install script on the kandev host. Progress streams
          live; you can start multiple installs in parallel.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
        {notInstalledAgents.map((agent) => (
          <InstallCard
            key={agent.name}
            agent={agent}
            copiedValue={copiedValue}
            onCopy={onCopy}
            job={installJobs[agent.name]}
            onInstall={onInstall}
          />
        ))}
        {notInstalledTools.map((tool) => (
          <ToolInstallCard key={tool.name} tool={tool} copiedValue={copiedValue} onCopy={onCopy} />
        ))}
      </div>
    </div>
  );
}

type AgentProfilesSectionProps = {
  savedAgents: Agent[];
};

function AgentProfilesSection({ savedAgents }: AgentProfilesSectionProps) {
  if (!savedAgents.some((agent: Agent) => agent.profiles.length > 0)) {
    return null;
  }

  return (
    <div className="space-y-4">
      <Separator />
      <div>
        <h3 className="text-lg font-semibold">Agent Profiles</h3>
        <p className="text-sm text-muted-foreground">Manage existing profiles by agent.</p>
      </div>

      <div className="space-y-2">
        {savedAgents.flatMap((agent: Agent) =>
          agent.profiles.map((profile: AgentProfile) => (
            <ProfileListItem key={profile.id} agent={agent} profile={profile} />
          )),
        )}
      </div>
    </div>
  );
}

/**
 * Install state is held in the store (driven by WS events
 * agent.install.{started,output,finished}). This hook:
 *   - Rehydrates jobs on mount so a page reload picks up in-flight installs.
 *   - Subscribes to agent.available.updated → calls onSuccess() to rescan.
 *   - Exposes handleInstall(name) which POSTs to enqueue (idempotent on the
 *     server: clicking again while running returns the same job_id).
 */
function useInstallAgent(onSuccess: () => Promise<void>) {
  const installJobs = useAppStore((state) => state.installJobs.byAgent);
  const upsertInstallJob = useAppStore((state) => state.upsertInstallJob);

  useEffect(() => {
    let cancelled = false;
    listInstallJobs()
      .then((resp) => {
        if (cancelled) return;
        // Upsert per-job rather than wholesale-replace: if a WS event
        // already seeded an in-flight job with output chunks between page
        // mount and this HTTP response, the snapshot from the server may
        // be older, and a full replace would clobber the live output.
        for (const job of resp.jobs) upsertInstallJob(job);
      })
      .catch(() => {
        /* page mount; ignore */
      });
    return () => {
      cancelled = true;
    };
  }, [upsertInstallJob]);

  // When any install finishes successfully, trigger the page-level rescan so
  // the agent disappears from "Available to Install" and shows up under
  // "Installed Agents".
  useEffect(() => {
    const succeeded = Object.values(installJobs).filter((j) => j.status === "succeeded");
    if (succeeded.length > 0) {
      void onSuccess();
    }
    // Intentionally only depends on the count of succeeded jobs to avoid
    // re-firing on every output chunk during a running install.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [Object.values(installJobs).filter((j) => j.status === "succeeded").length]);

  const handleInstall = useCallback(
    async (name: string) => {
      try {
        const job = await installAgent(name);
        // The WS event will normally arrive first, but seed the store in case
        // the WS round-trip is slower than the HTTP response.
        upsertInstallJob(job);
      } catch (err) {
        upsertInstallJob({
          job_id: `local-error-${name}`,
          agent_name: name,
          status: "failed",
          error: err instanceof Error ? err.message : String(err),
          started_at: new Date().toISOString(),
        });
      }
    },
    [upsertInstallJob],
  );

  return { installJobs, handleInstall };
}

function useAgentPageState() {
  const { items: discoveryAgents, loading: discoveryLoading } = useAgentDiscovery();
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const { items: availableAgents, tools } = useAvailableAgents();
  const [rescanning, setRescanning] = useState(false);
  const [tuiDialogOpen, setTuiDialogOpen] = useState(false);

  const installedAgents = useMemo(
    () => discoveryAgents.filter((agent: AgentDiscovery) => agent.available),
    [discoveryAgents],
  );
  const notInstalledAgents = useMemo(
    () => availableAgents.filter((a: AvailableAgent) => !a.available && a.install_script),
    [availableAgents],
  );
  const savedAgentsByName = useMemo(
    () => new Map(savedAgents.map((agent: Agent) => [agent.name, agent])),
    [savedAgents],
  );
  const resolveDisplayName = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.display_name ?? name;
  const resolveCapabilityStatus = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.model_config?.status;

  const handleRescan = async () => {
    if (rescanning) {
      return;
    }
    setRescanning(true);
    try {
      const [discoveryResp, availableResp] = await Promise.all([
        listAgentDiscovery({ cache: "no-store" }),
        listAvailableAgents({ cache: "no-store" }),
      ]);
      setAgentDiscovery(discoveryResp.agents);
      setAvailableAgents(availableResp.agents, availableResp.tools ?? []);
    } finally {
      setRescanning(false);
    }
  };

  const { installJobs, handleInstall } = useInstallAgent(handleRescan);

  const handleCreateCustomTUI = async (data: {
    display_name: string;
    model?: string;
    command: string;
  }) => {
    await createCustomTUIAgent(data);
    const [discoveryResp, agentsResp, availableResp] = await Promise.all([
      listAgentDiscovery({ cache: "no-store" }),
      listAgents({ cache: "no-store" }),
      listAvailableAgents({ cache: "no-store" }),
    ]);
    setAgentDiscovery(discoveryResp.agents);
    setSettingsAgents(agentsResp.agents);
    setAgentProfiles(
      agentsResp.agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
    );
    setAvailableAgents(availableResp.agents, availableResp.tools ?? []);
  };

  return {
    savedAgents,
    installedAgents,
    notInstalledAgents,
    tools,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    resolveCapabilityStatus,
    handleRescan,
    handleCreateCustomTUI,
    installJobs,
    handleInstall,
  };
}

export default function AgentsSettingsPage() {
  const {
    savedAgents,
    installedAgents,
    notInstalledAgents,
    tools,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    resolveCapabilityStatus,
    handleRescan,
    handleCreateCustomTUI,
    installJobs,
    handleInstall,
  } = useAgentPageState();
  const { copiedValue, copy } = useCopyCommand();

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Agents</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Discover installed agents, install new ones, and manage their profiles.
        </p>
      </div>

      <Separator />

      <InstalledAgentsSection
        installedAgents={installedAgents}
        discoveryLoading={discoveryLoading}
        rescanning={rescanning}
        savedAgentsByName={savedAgentsByName}
        resolveDisplayName={resolveDisplayName}
        resolveCapabilityStatus={resolveCapabilityStatus}
        setTuiDialogOpen={setTuiDialogOpen}
        handleRescan={handleRescan}
      />

      <SuggestInstallSection
        notInstalledAgents={notInstalledAgents}
        tools={tools}
        copiedValue={copiedValue}
        onCopy={copy}
        installJobs={installJobs}
        onInstall={handleInstall}
      />

      <AgentProfilesSection savedAgents={savedAgents} />

      <AddTUIAgentDialog
        open={tuiDialogOpen}
        onOpenChange={setTuiDialogOpen}
        onSubmit={handleCreateCustomTUI}
      />
    </div>
  );
}
