"use client";

import { useState } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Switch } from "@kandev/ui/switch";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconChevronDown, IconAlertTriangle } from "@tabler/icons-react";
import type {
  AgentRoutingOverrides,
  Tier,
  TierPerReason,
  WakeReason,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import {
  USE_AGENT_TIER,
  WAKE_REASONS,
} from "../../../workspace/routing/components/wake-reason-info";

type Props = {
  overrides: AgentRoutingOverrides;
  setOverrides: (next: AgentRoutingOverrides) => void;
  workspaceConfig: WorkspaceRouting | undefined;
};

// AgentWakeReasonOverrides is the per-agent override surface for the
// wake-reason tier policy. Collapsed by default — most agents inherit
// the workspace policy. When expanded, the user can flip a switch to
// override and pick a tier (or "use agent's normal tier") per reason.
export function AgentWakeReasonOverrides({ overrides, setOverrides, workspaceConfig }: Props) {
  const isOverriding = overrides.tier_per_reason_source === "override";
  const [open, setOpen] = useState(isOverriding);
  const wsPolicy = workspaceConfig?.tier_per_reason ?? {};
  const agentMap = overrides.tier_per_reason ?? {};
  const handleToggle = (on: boolean) => {
    if (on) {
      setOverrides({
        ...overrides,
        tier_per_reason_source: "override",
        tier_per_reason: { ...wsPolicy, ...agentMap },
      });
    } else {
      setOverrides({
        ...overrides,
        tier_per_reason_source: "inherit",
        tier_per_reason: {},
      });
    }
  };
  const handleRowChange = (reason: WakeReason, tier: Tier | typeof USE_AGENT_TIER) => {
    const next: TierPerReason = { ...agentMap };
    if (tier === USE_AGENT_TIER) {
      delete next[reason];
    } else {
      next[reason] = tier;
    }
    setOverrides({ ...overrides, tier_per_reason: next });
  };
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-md border border-border">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center justify-between p-3 cursor-pointer hover:bg-muted/50"
        >
          <div className="text-left space-y-0.5">
            <p className="text-sm font-medium">Override wake-reason tiers</p>
            <InheritedSummary wsPolicy={wsPolicy} overriding={isOverriding} />
          </div>
          <IconChevronDown
            className={`h-4 w-4 text-muted-foreground transition-transform ${
              open ? "rotate-180" : ""
            }`}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 border-t border-border p-3">
        <ToggleHeader checked={isOverriding} onChange={handleToggle} />
        {isOverriding && workspaceConfig && (
          <OverrideTable
            wsPolicy={wsPolicy}
            agentMap={agentMap}
            workspaceConfig={workspaceConfig}
            onChange={handleRowChange}
          />
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

function ToggleHeader({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <span className="text-sm">Override workspace policy for this agent</span>
        <Switch checked={checked} onCheckedChange={onChange} className="cursor-pointer" />
      </div>
      <p className="text-xs text-muted-foreground leading-relaxed">
        This is rare. Override only if this agent specifically needs different behaviour for its
        background work — for example, a security-critical agent that must use Frontier even for
        routine checks. Most agents should inherit the workspace policy.
      </p>
    </div>
  );
}

function InheritedSummary({
  wsPolicy,
  overriding,
}: {
  wsPolicy: TierPerReason;
  overriding: boolean;
}) {
  if (overriding) {
    return (
      <p className="text-xs text-muted-foreground">
        Using this agent&apos;s wake-reason overrides.
      </p>
    );
  }
  const parts = WAKE_REASONS.map((r) => {
    const t = wsPolicy[r.id];
    if (!t) return null;
    return `${r.label} → ${t}`;
  }).filter((s): s is string => s !== null);
  if (parts.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        Inherits workspace policy (no wake-reason tiers set yet).
      </p>
    );
  }
  return (
    <p className="text-xs text-muted-foreground">Inherits workspace policy: {parts.join(", ")}.</p>
  );
}

const TIER_OPTIONS: Array<{ value: Tier; label: string }> = [
  { value: "frontier", label: "Frontier" },
  { value: "balanced", label: "Balanced" },
  { value: "economy", label: "Economy" },
];

type OverrideTableProps = {
  wsPolicy: TierPerReason;
  agentMap: TierPerReason;
  workspaceConfig: WorkspaceRouting;
  onChange: (reason: WakeReason, tier: Tier | typeof USE_AGENT_TIER) => void;
};

function OverrideTable({ wsPolicy, agentMap, workspaceConfig, onChange }: OverrideTableProps) {
  return (
    <div className="divide-y divide-border">
      {WAKE_REASONS.map((row) => {
        const tier = agentMap[row.id];
        const inheritedTier = wsPolicy[row.id];
        return (
          <div key={row.id} className="py-2 space-y-1.5 first:pt-0 last:pb-0">
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs font-medium uppercase tracking-wide">{row.label}</span>
              <Select
                value={tier ?? USE_AGENT_TIER}
                onValueChange={(v) => onChange(row.id, v as Tier | typeof USE_AGENT_TIER)}
              >
                <SelectTrigger className="w-[220px] cursor-pointer">
                  <SelectValue placeholder="Use agent's normal tier" />
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
            </div>
            <p className="text-[11px] text-muted-foreground pl-1 leading-relaxed">{row.short}</p>
            {inheritedTier && !tier && (
              <p className="text-[11px] text-muted-foreground/80 pl-1">
                Workspace default for this reason:{" "}
                <Badge variant="secondary" className="capitalize">
                  {inheritedTier}
                </Badge>
              </p>
            )}
            <UnmappedWarning tier={tier} config={workspaceConfig} />
          </div>
        );
      })}
    </div>
  );
}

function UnmappedWarning({ tier, config }: { tier: Tier | undefined; config: WorkspaceRouting }) {
  if (!tier) return null;
  const order =
    config.provider_order && config.provider_order.length > 0 ? config.provider_order : [];
  for (const providerId of order) {
    const m = config.provider_profiles?.[providerId]?.tier_map?.[tier];
    if (m) return null;
  }
  return (
    <p className="text-[11px] text-destructive flex items-center gap-1 pl-1">
      <IconAlertTriangle className="h-3 w-3" />
      No provider in the effective order has {tier} mapped.
    </p>
  );
}
