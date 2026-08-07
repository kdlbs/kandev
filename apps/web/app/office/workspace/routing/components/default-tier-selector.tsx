"use client";

import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { Tier } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// `value`s are the wire `Tier` ids.
const TIER_OPTIONS: Array<{ value: Tier; labelKey: string; hintKey: string }> = [
  { value: "frontier", labelKey: "office:tierFrontier", hintKey: "office:tierFrontierHint" },
  { value: "balanced", labelKey: "office:tierBalanced", hintKey: "office:tierBalancedHint" },
  { value: "economy", labelKey: "office:tierEconomy", hintKey: "office:tierEconomyHint" },
];

type Props = {
  value: Tier;
  onChange: (v: Tier) => void;
  disabled?: boolean;
};

export function DefaultTierSelector({ value, onChange, disabled }: Props) {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div>
        <p className="text-sm font-medium">{t("office:defaultTier")}</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          {t("office:agentsInheritThisTierUnlessThey")}
        </p>
      </div>
      <ToggleGroup
        type="single"
        value={value}
        onValueChange={(v) => v && onChange(v as Tier)}
        disabled={disabled}
        className="justify-start"
      >
        {TIER_OPTIONS.map((opt) => (
          <ToggleGroupItem
            key={opt.value}
            value={opt.value}
            className="cursor-pointer flex flex-col items-center px-4 py-2 h-auto"
            title={t(opt.hintKey)}
          >
            <span className="text-sm">{t(opt.labelKey)}</span>
            <span className="text-[10px] text-muted-foreground">{t(opt.hintKey)}</span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
