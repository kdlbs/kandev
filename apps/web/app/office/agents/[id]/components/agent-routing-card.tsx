"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import type {
  AgentRoutingOverrides,
  Tier,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import { ProviderOrderEditor } from "../../../workspace/routing/components/provider-order-editor";
import { AgentWakeReasonOverrides } from "./agent-wake-reason-overrides";

const TIERS: Tier[] = ["frontier", "balanced", "economy"];

type Props = {
  agentId: string;
  /**
   * Override the initial form state. Falls back to the persisted
   * overrides from GET /agents/:id/route once that response lands, so
   * callers that don't pre-fetch can omit this prop entirely.
   */
  initial?: AgentRoutingOverrides;
};

const DEFAULT_INHERIT: AgentRoutingOverrides = {
  tier_source: "inherit",
  provider_order_source: "inherit",
};

export function AgentRoutingCard({ agentId, initial }: Props) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspace = useWorkspaceRouting(workspaceId);
  const route = useAgentRoute(agentId);
  // Hydrate the form from (in priority order) an explicit `initial`
  // prop, the persisted overrides on the route data, or the default
  // inherit markers. Using the persisted overrides means the toggles +
  // tier override + provider-order override reflect saved state on
  // first paint instead of always defaulting to "inherit".
  const persistedOverrides = route.data?.overrides;
  const [overrides, setOverrides] = useState<AgentRoutingOverrides>(
    initial ?? persistedOverrides ?? DEFAULT_INHERIT,
  );
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (initial) {
      setOverrides(initial);
    } else if (persistedOverrides) {
      setOverrides(persistedOverrides);
    }
  }, [initial, persistedOverrides]);

  if (!workspace.config?.enabled) {
    return <DisabledCard />;
  }

  const handleSave = async () => {
    setSaving(true);
    try {
      await route.updateOverrides(overrides);
      toast.success("Routing overrides saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const tierWarning = tierMissingMappingWarning(overrides, workspace.config);

  return (
    <Card>
      <Header />
      <CardContent className="space-y-4">
        <RoutingFields
          overrides={overrides}
          setOverrides={setOverrides}
          workspaceConfig={workspace.config}
          knownProviders={workspace.knownProviders}
          saving={saving}
        />
        <AgentWakeReasonOverrides
          overrides={overrides}
          setOverrides={setOverrides}
          workspaceConfig={workspace.config}
        />
        {tierWarning && (
          <p className="text-xs text-destructive" role="alert">
            {tierWarning}
          </p>
        )}
        <div className="flex justify-end">
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving || tierWarning !== null}
            className="cursor-pointer"
          >
            {saving ? "Saving…" : "Save overrides"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// tierMissingMappingWarning mirrors the server's
// ValidateAgentOverridesAgainstWorkspace check so the user gets an
// inline signal (and a disabled Save button) instead of saving a
// broken config and bouncing on a 400. Returns null when the chosen
// tier is mapped on at least one provider in the effective order.
function tierMissingMappingWarning(
  overrides: AgentRoutingOverrides,
  cfg: WorkspaceRouting | undefined,
): string | null {
  if (!cfg) return null;
  if (overrides.tier_source !== "override") return null;
  const tier = overrides.tier;
  if (!tier) return null;
  const order =
    overrides.provider_order_source === "override" && overrides.provider_order
      ? overrides.provider_order
      : cfg.provider_order;
  for (const providerId of order) {
    const profile = cfg.provider_profiles?.[providerId];
    if (!profile) continue;
    if (profile.tier_map?.[tier]) return null;
  }
  return `No provider in the effective order has the ${tier} tier mapped. Configure a model for ${tier} on at least one provider in Workspace → Provider routing.`;
}

function DisabledCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Provider routing</CardTitle>
        <p className="text-xs text-muted-foreground">
          Provider routing is off for this workspace. Enable it in Workspace → Provider routing to
          override the agent&apos;s tier or fallback chain.
        </p>
      </CardHeader>
    </Card>
  );
}

function Header() {
  return (
    <CardHeader>
      <CardTitle className="text-sm">Provider routing</CardTitle>
      <p className="text-xs text-muted-foreground">
        Override the workspace&apos;s tier or fallback order for this agent. Inheriting keeps the
        agent in step with workspace settings.
      </p>
    </CardHeader>
  );
}

type FieldsProps = {
  overrides: AgentRoutingOverrides;
  setOverrides: (next: AgentRoutingOverrides) => void;
  workspaceConfig: WorkspaceRouting | undefined;
  knownProviders: string[];
  saving: boolean;
};

function RoutingFields({
  overrides,
  setOverrides,
  workspaceConfig,
  knownProviders,
  saving,
}: FieldsProps) {
  const overrideTier = overrides.tier_source === "override";
  const overrideOrder = overrides.provider_order_source === "override";

  const setTierSource = (on: boolean) =>
    setOverrides({
      ...overrides,
      tier_source: on ? "override" : "inherit",
      tier: on ? overrides.tier || workspaceConfig?.default_tier || "balanced" : "",
    });
  const setOrderSource = (on: boolean) => {
    const next = computeNextOrder(on, overrides.provider_order, workspaceConfig?.provider_order);
    setOverrides({
      ...overrides,
      provider_order_source: on ? "override" : "inherit",
      provider_order: next,
    });
  };

  return (
    <>
      <InheritRow label="Override workspace tier" checked={overrideTier} onChange={setTierSource} />
      {overrideTier ? (
        <TierToggleGroup
          value={overrides.tier || ""}
          onChange={(t) => setOverrides({ ...overrides, tier: t })}
        />
      ) : (
        <InheritedTierHint defaultTier={workspaceConfig?.default_tier} />
      )}
      <InheritRow
        label="Override workspace provider order"
        checked={overrideOrder}
        onChange={setOrderSource}
      />
      {overrideOrder && (
        <ProviderOrderEditor
          order={overrides.provider_order ?? []}
          knownProviders={knownProviders}
          onChange={(next) => setOverrides({ ...overrides, provider_order: next })}
          disabled={saving}
        />
      )}
    </>
  );
}

function TierToggleGroup({ value, onChange }: { value: string; onChange: (t: Tier) => void }) {
  return (
    <ToggleGroup
      type="single"
      value={value}
      onValueChange={(v) => v && onChange(v as Tier)}
      className="justify-start"
    >
      {TIERS.map((t) => (
        <ToggleGroupItem key={t} value={t} className="cursor-pointer capitalize">
          {t}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

function InheritedTierHint({ defaultTier }: { defaultTier?: Tier }) {
  if (!defaultTier) return null;
  return (
    <p className="text-xs text-muted-foreground">
      Inherits{" "}
      <Badge variant="secondary" className="capitalize">
        {defaultTier}
      </Badge>{" "}
      from workspace.
    </p>
  );
}

function computeNextOrder(
  on: boolean,
  current: string[] | undefined,
  workspaceOrder: string[] | undefined,
): string[] {
  if (!on) return [];
  if (current && current.length > 0) return current;
  return workspaceOrder ?? [];
}

function InheritRow({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-sm">{label}</span>
      <Switch checked={checked} onCheckedChange={onChange} className="cursor-pointer" />
    </div>
  );
}
