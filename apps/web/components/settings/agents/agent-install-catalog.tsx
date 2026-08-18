"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconClipboard, IconDownload, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { InstallAgentCard } from "@/components/settings/install-agent-card";
import { useAgentDiscovery } from "@/hooks/domains/settings/use-agent-discovery";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { installAgent, listAgentDiscovery, listAvailableAgents, listInstallJobs } from "@/lib/api";
import type { InstallJob } from "@/lib/api";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import type { AvailableAgent, ToolStatus } from "@/lib/types/http";

function useCopyCommand() {
  const [copiedValue, setCopiedValue] = useState<string | null>(null);
  const copy = useCallback(async (text: string) => {
    if (await copyToClipboard(text)) {
      setCopiedValue(text);
      setTimeout(() => setCopiedValue(null), 2000);
    }
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
  const { t } = useTranslation();
  const isCopied = copiedValue === text;
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-7 w-7 p-0 cursor-pointer shrink-0"
      aria-label={isCopied ? t("agents:copied") : t("agents:copyInstallCommand")}
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
  const { t } = useTranslation();
  return (
    <Card className="border-dashed">
      <CardContent className="py-4 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <IconDownload className="h-5 w-5 text-muted-foreground shrink-0" />
          <h4 className="font-medium">{tool.display_name}</h4>
          {tool.available && (
            <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
              <IconCheck className="h-3.5 w-3.5" />
              {t("agents:installed")}
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

/**
 * Install state is held in the store (driven by WS events
 * agent.install.{started,output,finished}). This hook:
 *   - Rehydrates jobs on mount so a page reload picks up in-flight installs.
 *   - Calls onSuccess() when an install finishes so the catalogue refreshes.
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

  // When any install finishes successfully, refresh so the agent disappears
  // from the catalogue and shows up under "Installed Agents".
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

/** The install catalogue: every not-yet-installed agent and companion tool. */
export function AgentInstallCatalog() {
  const { t } = useTranslation();
  const { copiedValue, copy } = useCopyCommand();
  const { items: availableAgents, tools } = useAvailableAgents();
  useAgentDiscovery();
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);

  const refresh = useCallback(async () => {
    const [discoveryResp, availableResp] = await Promise.all([
      listAgentDiscovery({ cache: "no-store" }),
      listAvailableAgents({ cache: "no-store" }),
    ]);
    setAgentDiscovery(discoveryResp.agents);
    setAvailableAgents(availableResp.agents, availableResp.tools ?? []);
  }, [setAgentDiscovery, setAvailableAgents]);

  const { installJobs, handleInstall } = useInstallAgent(refresh);

  const notInstalledAgents = useMemo(
    () => availableAgents.filter((a: AvailableAgent) => !a.available && a.install_script),
    [availableAgents],
  );
  const notInstalledTools = tools.filter((tool: ToolStatus) => !tool.available);

  if (notInstalledAgents.length === 0 && notInstalledTools.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center">
          <p className="text-sm text-muted-foreground">{t("agents:everythingInstalled")}</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
      {notInstalledAgents.map((agent) => (
        <InstallCard
          key={agent.name}
          agent={agent}
          copiedValue={copiedValue}
          onCopy={copy}
          job={installJobs[agent.name]}
          onInstall={handleInstall}
        />
      ))}
      {notInstalledTools.map((tool) => (
        <ToolInstallCard key={tool.name} tool={tool} copiedValue={copiedValue} onCopy={copy} />
      ))}
    </div>
  );
}
