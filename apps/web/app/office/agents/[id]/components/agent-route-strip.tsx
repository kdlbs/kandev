"use client";

import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";
import { useAppStore } from "@/components/state-provider";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { providerLabel } from "../../../workspace/routing/components/provider-order-editor";
import { TIER_SOURCE_LABEL_KEYS } from "../../../lib/label-keys";
import { useTranslation } from "react-i18next";

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
// The record keys are wire values and stay untranslated.
const CONFIGURED_TOOLTIP = "office:configuredRouteTooltip";
const CURRENT_TOOLTIP = "office:currentRouteTooltip";

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
  const { t } = useTranslation();
  const primaryProvider = preview.primary_provider_id ?? "";
  const primaryModel = preview.primary_model ?? "";
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={`${LABEL_CLASS} cursor-help`}>{t("office:configured")}</span>
        </TooltipTrigger>
        <TooltipContent>{t(CONFIGURED_TOOLTIP)}</TooltipContent>
      </Tooltip>
      <span className="font-mono">
        {primaryProvider === "" ? (
          <span className="italic">{t("office:noneLower")}</span>
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
      <div className="flex items-center gap-1.5 ml-auto">
        <Badge variant="secondary" className="capitalize">
          {preview.effective_tier}
        </Badge>
        <span className={LABEL_CLASS}>{t(TIER_SOURCE_LABEL_KEYS[preview.tier_source])}</span>
      </div>
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={`${LABEL_CLASS} cursor-help`}>{t("office:current")}</span>
        </TooltipTrigger>
        <TooltipContent>{t(CURRENT_TOOLTIP)}</TooltipContent>
      </Tooltip>
      <span className="font-mono">
        {providerLabel(provider)}/{model || "?"}
      </span>
      {failureCode && <Badge variant="destructive">{failureCode}</Badge>}
    </div>
  );
}
