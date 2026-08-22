"use client";

import { useEffect, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import type {
  AgentRoutePreview,
  AgentRoutingOverrides,
  Tier,
  TierPerReason,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import { ProviderOrderEditor } from "../../../workspace/routing/components/provider-order-editor";
import { AgentWakeReasonOverrides } from "./agent-wake-reason-overrides";
import { WAKE_REASONS } from "../../../workspace/routing/components/wake-reason-info";
import type { TFunction } from "i18next";
import { Trans, useTranslation } from "react-i18next";
import { TIER_NAME_KEYS } from "../../../lib/label-keys";

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
  const { t } = useTranslation();
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
    return null;
  }

  const handleSave = async () => {
    setSaving(true);
    try {
      await route.updateOverrides(overrides);
      toast.success(t("office:routingOverridesSaved"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSave"));
    } finally {
      setSaving(false);
    }
  };

  const tierWarning = tierMissingMappingWarning(t, overrides, workspace.config);

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
          preview={route.data?.preview}
        />
        <TierShadowingNotice
          overrides={overrides}
          workspaceConfig={workspace.config}
          preview={route.data?.preview}
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
            {saving ? t("office:savingEllipsis") : t("office:saveOverrides")}
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
  t: TFunction,
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
    const executionProfileIDs = profile.execution_profile_ids ?? profile.tier_profile_ids;
    if (executionProfileIDs?.[tier]) return null;
  }
  // One key: the tier appears twice in the sentence, so fragments would freeze
  // both the order and the repetition.
  return t("office:noProviderMappedForTierHelp", { tier: t(TIER_NAME_KEYS[tier]) });
}

function Header() {
  const { t } = useTranslation();
  return (
    <CardHeader>
      <CardTitle className="text-sm">{t("office:providerRoutingHeading")}</CardTitle>
      <p className="text-xs text-muted-foreground">
        {t("office:overrideTheWorkspaceTierOrProvider")}
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
  preview: AgentRoutePreview | undefined;
};

function RoutingFields({
  overrides,
  setOverrides,
  workspaceConfig,
  knownProviders,
  saving,
  preview,
}: FieldsProps) {
  const { t } = useTranslation();
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
      <InheritRow
        label={t("office:overrideWorkspaceTier")}
        checked={overrideTier}
        onChange={setTierSource}
      />
      {overrideTier ? (
        <TierToggleGroup
          value={overrides.tier || ""}
          onChange={(t) => setOverrides({ ...overrides, tier: t })}
        />
      ) : (
        <InheritedTierHint preview={preview} defaultTier={workspaceConfig?.default_tier} />
      )}
      <InheritRow
        label={t("office:overrideWorkspaceProviderOrder")}
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

// AC-16: names both the tier in force and the level that supplied it. The
// preview's tier_source is authoritative (role vs. workspace); while it is
// still loading, fall back to the workspace default with "workspace"
// wording, since that is the only level we can name without it.
function InheritedTierHint({
  preview,
  defaultTier,
}: {
  preview: AgentRoutePreview | undefined;
  defaultTier?: Tier;
}) {
  const { t } = useTranslation();
  const tier = preview?.effective_tier ?? defaultTier;
  if (!tier) return null;
  const i18nKey =
    preview?.tier_source === "role"
      ? "office:inheritsTierFromRole"
      : "office:inheritsTierFromWorkspace";
  return (
    // One sentence, one key: "Inherits" + a bare tier id + "from workspace"
    // froze the English order and rendered the wire value as display copy.
    <p className="text-xs text-muted-foreground">
      <Trans i18nKey={i18nKey} values={{ tier: t(TIER_NAME_KEYS[tier]) }}>
        Inherits
        <Badge variant="secondary" className="capitalize">
          tier
        </Badge>
        from workspace.
      </Trans>
    </p>
  );
}

// AC-17 / AC-17a / AC-17b: names the wake reasons whose policy shadows the
// tier displayed above, whichever level supplied it. The effective map is
// the agent's own tier_per_reason when it overrides the workspace policy
// (an override replaces the map entirely, per docs/specs/office/routing.md)
// and the workspace map otherwise; keys with an empty value do not shadow
// anything and are excluded.
function effectiveWakeReasonMap(
  overrides: AgentRoutingOverrides,
  workspaceConfig: WorkspaceRouting | undefined,
): TierPerReason {
  if (overrides.tier_per_reason_source === "override") {
    return overrides.tier_per_reason ?? {};
  }
  return workspaceConfig?.tier_per_reason ?? {};
}

function TierShadowingNotice({
  overrides,
  workspaceConfig,
  preview,
}: {
  overrides: AgentRoutingOverrides;
  workspaceConfig: WorkspaceRouting | undefined;
  preview: AgentRoutePreview | undefined;
}) {
  const { t } = useTranslation();
  const overriding = overrides.tier_source === "override";
  // AC-18b: a preview never reports wake_reason, so the role/workspace case
  // is only reachable once the preview has actually loaded.
  const roleOrWorkspace = preview?.tier_source === "role" || preview?.tier_source === "workspace";
  if (!overriding && !roleOrWorkspace) return null;

  const map = effectiveWakeReasonMap(overrides, workspaceConfig);
  const reasons = WAKE_REASONS.filter((r) => !!map[r.id]).map((r) => t(r.labelKey));
  if (reasons.length === 0) return null;

  const i18nKey = overriding
    ? "office:overrideShadowedByWakeReasons"
    : "office:tierShadowedByWakeReasons";
  return (
    <p className="text-xs text-muted-foreground" role="note">
      {t(i18nKey, { list: reasons.join(", ") })}
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
