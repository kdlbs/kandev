"use client";

import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type {
  ExecutionProfileSummary,
  ProviderProfile,
  Tier,
  TierMap,
} from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";

const UNMAPPED = "__unmapped__";
const TIERS: Array<{ key: keyof TierMap; label: Tier }> = [
  { key: "frontier", label: "frontier" },
  { key: "balanced", label: "balanced" },
  { key: "economy", label: "economy" },
];

type Props = {
  providerId: string;
  profile: ProviderProfile;
  executionProfiles: ExecutionProfileSummary[];
  defaultTier: Tier;
  onChange: (next: ProviderProfile) => void;
  disabled?: boolean;
};

export function ProviderTierMapping(props: Props) {
  const available = profilesForProvider(props.executionProfiles, props.providerId);
  const selectedIDs = props.profile.execution_profile_ids ?? props.profile.tier_profile_ids ?? {};

  const setTier = (tier: Tier, profileId: string) => {
    props.onChange(applyExecutionProfileSelection(props.profile, available, tier, profileId));
  };

  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm font-medium">{providerLabel(props.providerId)}</p>
        <span className="text-xs text-muted-foreground font-mono">{props.providerId}</span>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        {TIERS.map(({ key, label }) => {
          const value = selectedIDs[label] ?? UNMAPPED;
          return (
            <div key={key} className="min-w-0">
              <div className="flex items-center gap-1 mb-1">
                <Label className="text-xs uppercase">{label}</Label>
                {label === props.defaultTier && value === UNMAPPED && (
                  <Badge variant="destructive" className="text-[10px]">
                    Required
                  </Badge>
                )}
              </div>
              <Select
                value={value}
                onValueChange={(next) => setTier(label, next)}
                disabled={props.disabled}
              >
                <SelectTrigger
                  className="w-full cursor-pointer"
                  data-testid={`tier-profile-${props.providerId}-${label}`}
                >
                  <SelectValue placeholder="Select execution profile" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={UNMAPPED}>Not configured</SelectItem>
                  {available.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.name} · {item.model}
                      {item.mode ? ` · ${item.mode}` : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          );
        })}
      </div>
      {available.length === 0 && (
        <p className="text-xs text-destructive">
          No active execution profiles are available for this provider.
        </p>
      )}
    </div>
  );
}

export function profilesForProvider(
  profiles: ExecutionProfileSummary[],
  providerId: string,
): ExecutionProfileSummary[] {
  return profiles.filter((item) => item.provider_id === providerId);
}

export function applyExecutionProfileSelection(
  profile: ProviderProfile,
  available: ExecutionProfileSummary[],
  tier: Tier,
  profileId: string,
): ProviderProfile {
  const selected = available.find((item) => item.id === profileId);
  const selectedIDs = profile.execution_profile_ids ?? profile.tier_profile_ids ?? {};
  return {
    ...profile,
    tier_map: { ...profile.tier_map, [tier]: selected?.model },
    execution_profile_ids: { ...selectedIDs, [tier]: selected?.id },
    tier_profile_ids: undefined,
    mode: undefined,
    flags: undefined,
    env: undefined,
  };
}
