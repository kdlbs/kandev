"use client";

import { Switch } from "@kandev/ui/switch";

type Props = {
  enabled: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
};

export function RoutingEnableCard({ enabled, onChange, disabled }: Props) {
  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium">Automatic provider fallback</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            When enabled, provider limits can move a run to the next configured execution profile.
            When disabled, Office uses only the first provider for the selected tier.
          </p>
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={onChange}
          disabled={disabled}
          className="cursor-pointer"
        />
      </div>
    </div>
  );
}
