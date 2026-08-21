"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Badge } from "@kandev/ui/badge";
import { IconInfoCircle, IconAlertTriangle } from "@tabler/icons-react";
import type {
  AgentRole,
  RoleMeta,
  RoleTierMap,
  Tier,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";
import { TIER_NAME_KEYS } from "../../../lib/label-keys";
import { firstProviderWithTier } from "./wake-reason-tier-card";
import { useTranslation } from "react-i18next";

// USE_WORKSPACE_DEFAULT is the sentinel used in the per-role tier dropdown
// to mean "no role entry — use the workspace default tier." Persisting this
// removes the role's key from the workspace's role_tiers map.
const USE_WORKSPACE_DEFAULT = "__workspace_default__" as const;

type Props = {
  roles: RoleMeta[];
  config: WorkspaceRouting;
  value: RoleTierMap;
  onChange: (next: RoleTierMap) => void;
  disabled?: boolean;
};

// RoleTierCard renders the per-role tier policy table on the workspace
// routing settings page: the third of four precedence levels, below a
// wake-reason policy and a per-agent override, above the workspace default.
// See docs/specs/office-agent-tier-routing/spec.md.
export function RoleTierCard({ roles, config, value, onChange, disabled }: Props) {
  const { t } = useTranslation();
  const handleRowChange = (role: AgentRole, tier: Tier | typeof USE_WORKSPACE_DEFAULT) => {
    const next: RoleTierMap = { ...value };
    if (tier === USE_WORKSPACE_DEFAULT) {
      delete next[role];
    } else {
      next[role] = tier;
    }
    onChange(next);
  };
  if (roles.length === 0) return null;
  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="text-sm">{t("office:roleTierPolicy")}</CardTitle>
        <p className="text-xs text-muted-foreground leading-relaxed">
          {t("office:setADefaultTierForEveryAgentOfA")}
        </p>
      </CardHeader>
      <CardContent className="divide-y divide-border">
        {roles.map((role) => (
          <RoleRow
            key={role.id}
            role={role}
            tier={value[role.id as AgentRole]}
            config={config}
            disabled={disabled}
            onChange={(tier) => handleRowChange(role.id as AgentRole, tier)}
          />
        ))}
      </CardContent>
    </Card>
  );
}

type RowProps = {
  role: RoleMeta;
  tier: Tier | undefined;
  config: WorkspaceRouting;
  disabled?: boolean;
  onChange: (tier: Tier | typeof USE_WORKSPACE_DEFAULT) => void;
};

function RoleRow({ role, tier, config, disabled, onChange }: RowProps) {
  const labelId = `role-tier-label-${role.id}`;
  return (
    <div className="py-3 space-y-2 first:pt-0 last:pb-0">
      <div className="flex items-center justify-between gap-3">
        <RowLabel role={role} labelId={labelId} />
        <TierSelect tier={tier} onChange={onChange} disabled={disabled} ariaLabelledBy={labelId} />
      </div>
      <ResolvedRow tier={tier} config={config} />
    </div>
  );
}

function RowLabel({ role, labelId }: { role: RoleMeta; labelId: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1.5">
      <span id={labelId} className="text-sm font-medium">
        {role.label}
      </span>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("office:moreInfoAbout", { label: role.label })}
            className="inline-flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center text-muted-foreground hover:text-foreground"
          >
            <IconInfoCircle className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-xs text-xs leading-relaxed">
          {role.description || t("office:roleTierRowInfo")}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
const TIER_OPTIONS: Array<{ value: Tier; labelKey: string }> = [
  { value: "frontier", labelKey: "office:tierFrontierQualified" },
  { value: "balanced", labelKey: "office:tierBalancedQualified" },
  { value: "economy", labelKey: "office:tierEconomyQualified" },
];

function TierSelect({
  tier,
  onChange,
  disabled,
  ariaLabelledBy,
}: {
  tier: Tier | undefined;
  onChange: (tier: Tier | typeof USE_WORKSPACE_DEFAULT) => void;
  disabled?: boolean;
  ariaLabelledBy: string;
}) {
  const { t } = useTranslation();
  const selected = tier ?? USE_WORKSPACE_DEFAULT;
  return (
    <Select
      value={selected}
      onValueChange={(v) => onChange(v as Tier | typeof USE_WORKSPACE_DEFAULT)}
      disabled={disabled}
    >
      <SelectTrigger aria-labelledby={ariaLabelledBy} className="min-h-11 w-[220px] cursor-pointer">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={USE_WORKSPACE_DEFAULT} className="cursor-pointer">
          {t("office:useWorkspaceDefaultTier")}
        </SelectItem>
        {TIER_OPTIONS.map((opt) => (
          <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
            {t(opt.labelKey)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

// ResolvedRow shows "Currently maps to: <provider> / <model>" so users see
// the concrete (provider, model) their selection produces. When the
// selected tier is not mapped on any provider in the order we surface a
// warning chip directing them to the provider tier mapping section.
function ResolvedRow({ tier, config }: { tier: Tier | undefined; config: WorkspaceRouting }) {
  const { t } = useTranslation();
  if (!tier) return null;
  const match = firstProviderWithTier(tier, config);
  if (!match) {
    return (
      <p className="text-[11px] text-destructive flex items-center gap-1 pl-1">
        <IconAlertTriangle className="h-3 w-3" />
        {/* One key: the tier appeared twice in one sentence, so two fragments
            would have frozen both the order and the repetition. */}
        {t("office:noProviderHasTierMapped", { tier: t(TIER_NAME_KEYS[tier]) })}
      </p>
    );
  }
  return (
    <p className="text-[11px] text-muted-foreground pl-1">
      {t("office:currentlyMapsTo")}{" "}
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="secondary" className="ml-0.5 cursor-help">
            {providerLabel(match.providerId)} / {match.model}
          </Badge>
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-xs text-xs leading-relaxed">
          {t("office:typicalMappingAtLaunchTimeThe")}
        </TooltipContent>
      </Tooltip>
    </p>
  );
}
