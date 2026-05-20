"use client";

import { useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { ProviderProfile, Tier, TierMap } from "@/lib/state/slices/office/types";
import { providerLabel } from "./provider-order-editor";

const TIERS: Array<{ key: keyof TierMap; label: Tier }> = [
  { key: "frontier", label: "frontier" },
  { key: "balanced", label: "balanced" },
  { key: "economy", label: "economy" },
];

type Props = {
  providerId: string;
  profile: ProviderProfile;
  defaultTier: Tier;
  onChange: (next: ProviderProfile) => void;
  disabled?: boolean;
};

export function ProviderTierMapping({
  providerId,
  profile,
  defaultTier,
  onChange,
  disabled,
}: Props) {
  const setTier = (key: keyof TierMap, value: string) =>
    onChange({ ...profile, tier_map: { ...profile.tier_map, [key]: value } });

  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div className="flex items-center gap-2">
        <p className="text-sm font-medium">{providerLabel(providerId)}</p>
        <span className="text-xs text-muted-foreground font-mono">{providerId}</span>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        {TIERS.map(({ key, label }) => (
          <div key={key}>
            <div className="flex items-center gap-1 mb-1">
              <Label className="text-xs uppercase tracking-wide">{label}</Label>
              {label === defaultTier && !profile.tier_map[key] && (
                <Badge variant="destructive" className="text-[10px]">
                  Required
                </Badge>
              )}
            </div>
            <Input
              value={profile.tier_map[key] ?? ""}
              onChange={(e) => setTier(key, e.target.value)}
              placeholder={`${label} model id`}
              disabled={disabled}
              data-testid={`tier-input-${providerId}-${label}`}
            />
          </div>
        ))}
      </div>
      <AdvancedSection profile={profile} onChange={onChange} disabled={disabled} />
    </div>
  );
}

function AdvancedSection({
  profile,
  onChange,
  disabled,
}: {
  profile: ProviderProfile;
  onChange: (next: ProviderProfile) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="cursor-pointer text-xs gap-1 h-7"
        >
          {open ? (
            <IconChevronDown className="h-3 w-3" />
          ) : (
            <IconChevronRight className="h-3 w-3" />
          )}
          Advanced
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-2 grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <Label className="text-xs">Mode</Label>
          <Input
            value={profile.mode ?? ""}
            onChange={(e) => onChange({ ...profile, mode: e.target.value })}
            placeholder="default"
            disabled={disabled}
            className="mt-1"
          />
        </div>
        <div className="md:col-span-2">
          <Label className="text-xs">Extra CLI flags</Label>
          <Input
            value={(profile.flags ?? []).join(" ")}
            onChange={(e) =>
              onChange({
                ...profile,
                flags: e.target.value.trim() === "" ? undefined : e.target.value.split(/\s+/),
              })
            }
            placeholder="--space-separated --flags"
            disabled={disabled}
            className="mt-1"
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
