"use client";

import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";
import { useAppStore } from "@/components/state-provider";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { providerLabel } from "../../../workspace/routing/components/provider-order-editor";

const CONFIGURED_TOOLTIP = "Primary route from workspace + agent overrides.";
const CURRENT_TOOLTIP =
  "Reflects the next-launch choice. In-flight session details are on each run.";

type Props = { agentId: string };

export function AgentRouteStrip({ agentId }: Props) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspace = useWorkspaceRouting(workspaceId);
  const { data } = useAgentRoute(agentId);

  if (!workspace.config?.enabled) return null;
  if (!data) return null;

  const preview = data.preview;
  const primaryProvider = preview.primary_provider_id ?? "";
  const currentProvider = preview.current_provider_id ?? "";
  const fellBack =
    preview.degraded &&
    currentProvider !== "" &&
    primaryProvider !== "" &&
    currentProvider !== primaryProvider;

  return (
    <div className="rounded-lg border border-border p-3 space-y-1.5 text-xs">
      <ConfiguredRow preview={preview} />
      {fellBack && (
        <CurrentRow
          provider={currentProvider}
          model={preview.current_model ?? ""}
          failureCode={data.last_failure_code}
        />
      )}
    </div>
  );
}

const LABEL_CLASS = "text-muted-foreground uppercase tracking-wide";

function ConfiguredRow({ preview }: { preview: AgentRoutePreview }) {
  const primaryProvider = preview.primary_provider_id ?? "";
  const primaryModel = preview.primary_model ?? "";
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={`${LABEL_CLASS} cursor-help`}>Configured</span>
        </TooltipTrigger>
        <TooltipContent>{CONFIGURED_TOOLTIP}</TooltipContent>
      </Tooltip>
      <span className="font-mono">
        {primaryProvider === "" ? (
          <span className="italic">none</span>
        ) : (
          <>
            {providerLabel(primaryProvider)}/{primaryModel || "?"}
          </>
        )}
        {preview.fallback_chain.map((p, i) => (
          <span key={`${p.provider_id}-${i}`}>
            <span className="text-muted-foreground px-1">→</span>
            {providerLabel(p.provider_id)}/{p.model || "?"}
          </span>
        ))}
      </span>
      <Badge variant="secondary" className="capitalize ml-auto">
        {preview.effective_tier}
      </Badge>
    </div>
  );
}

function CurrentRow({
  provider,
  model,
  failureCode,
}: {
  provider: string;
  model: string;
  failureCode?: string;
}) {
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={`${LABEL_CLASS} cursor-help`}>Current</span>
        </TooltipTrigger>
        <TooltipContent>{CURRENT_TOOLTIP}</TooltipContent>
      </Tooltip>
      <span className="font-mono">
        {providerLabel(provider)}/{model || "?"}
      </span>
      {failureCode && <Badge variant="destructive">{failureCode}</Badge>}
    </div>
  );
}
