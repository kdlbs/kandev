"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Badge } from "@kandev/ui/badge";
import { IconInfoCircle, IconAlertTriangle } from "@tabler/icons-react";
import type {
  Tier,
  TierPerReason,
  WakeReason,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";
import { USE_AGENT_TIER, WAKE_REASONS, type WakeReasonCopy } from "./wake-reason-info";

type Props = {
  config: WorkspaceRouting;
  value: TierPerReason;
  onChange: (next: TierPerReason) => void;
  disabled?: boolean;
};

// WakeReasonTierCard renders the per-wake-reason tier policy table on
// the workspace routing settings page. Every row has visible helper
// text so a new user understands what the row controls before touching
// the dropdown.
export function WakeReasonTierCard({ config, value, onChange, disabled }: Props) {
  const handleRowChange = (reason: WakeReason, tier: Tier | typeof USE_AGENT_TIER) => {
    const next: TierPerReason = { ...value };
    if (tier === USE_AGENT_TIER) {
      delete next[reason];
    } else {
      next[reason] = tier;
    }
    onChange(next);
  };
  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="text-sm">Wake-reason tier policy</CardTitle>
        <p className="text-xs text-muted-foreground leading-relaxed">
          Override which model tier runs for specific kinds of agent work. Most users leave this on
          the workspace defaults — Economy for background tasks, Balanced for normal runs.
          Heartbeats, scheduled routines, and budget alerts run constantly in the background, so
          using a cheaper tier here can dramatically reduce cost without affecting the work that
          matters.
        </p>
      </CardHeader>
      <CardContent className="divide-y divide-border">
        {WAKE_REASONS.map((row) => (
          <WakeReasonRow
            key={row.id}
            row={row}
            tier={value[row.id]}
            config={config}
            disabled={disabled}
            onChange={(tier) => handleRowChange(row.id, tier)}
          />
        ))}
      </CardContent>
    </Card>
  );
}

type RowProps = {
  row: WakeReasonCopy;
  tier: Tier | undefined;
  config: WorkspaceRouting;
  disabled?: boolean;
  onChange: (tier: Tier | typeof USE_AGENT_TIER) => void;
};

function WakeReasonRow({ row, tier, config, disabled, onChange }: RowProps) {
  return (
    <div className="py-3 space-y-2 first:pt-0 last:pb-0">
      <div className="flex items-center justify-between gap-3">
        <RowLabel row={row} />
        <TierSelect tier={tier} onChange={onChange} disabled={disabled} />
      </div>
      <p className="text-xs text-muted-foreground leading-relaxed pl-1">{row.short}</p>
      <p className="text-[11px] text-muted-foreground/80 leading-relaxed pl-1">
        {row.recommendation}
      </p>
      <ResolvedRow tier={tier} config={config} />
    </div>
  );
}

function RowLabel({ row }: { row: WakeReasonCopy }) {
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-sm font-medium uppercase tracking-wide">{row.label}</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={`More info about ${row.label}`}
            className="cursor-pointer text-muted-foreground hover:text-foreground"
          >
            <IconInfoCircle className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-xs text-xs leading-relaxed">
          {row.long}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

const TIER_OPTIONS: Array<{ value: Tier; label: string }> = [
  { value: "frontier", label: "Frontier (best capability)" },
  { value: "balanced", label: "Balanced (standard)" },
  { value: "economy", label: "Economy (cheapest)" },
];

function TierSelect({
  tier,
  onChange,
  disabled,
}: {
  tier: Tier | undefined;
  onChange: (tier: Tier | typeof USE_AGENT_TIER) => void;
  disabled?: boolean;
}) {
  const selected = tier ?? USE_AGENT_TIER;
  return (
    <Select
      value={selected}
      onValueChange={(v) => onChange(v as Tier | typeof USE_AGENT_TIER)}
      disabled={disabled}
    >
      <SelectTrigger className="w-[220px] cursor-pointer">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={USE_AGENT_TIER} className="cursor-pointer">
          Use agent&apos;s normal tier
        </SelectItem>
        {TIER_OPTIONS.map((opt) => (
          <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

// ResolvedRow shows "Currently maps to: <provider> / <model>" so users
// see the concrete (provider, model) their selection produces. When the
// selected tier is not mapped on any provider in the order we surface a
// warning chip directing them to the provider tier mapping section.
function ResolvedRow({ tier, config }: { tier: Tier | undefined; config: WorkspaceRouting }) {
  if (!tier) return null;
  const match = firstProviderWithTier(tier, config);
  if (!match) {
    return (
      <p className="text-[11px] text-destructive flex items-center gap-1 pl-1">
        <IconAlertTriangle className="h-3 w-3" />
        No provider has the {tier} tier mapped — set a model for {tier} under Provider tier mapping
        below.
      </p>
    );
  }
  return (
    <p className="text-[11px] text-muted-foreground pl-1">
      Currently maps to{" "}
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="secondary" className="ml-0.5 cursor-help">
            {providerLabel(match.providerId)} / {match.model}
          </Badge>
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-xs text-xs leading-relaxed">
          Typical mapping. At launch time the router walks the provider order and picks the first
          healthy provider, so a degraded primary may cause this run to use a different
          provider/model than shown here.
        </TooltipContent>
      </Tooltip>
    </p>
  );
}

function firstProviderWithTier(
  tier: Tier,
  config: WorkspaceRouting,
): { providerId: string; model: string } | null {
  for (const providerId of config.provider_order) {
    const profile = config.provider_profiles[providerId];
    const model = profile?.tier_map?.[tier];
    if (model) return { providerId, model };
  }
  return null;
}
