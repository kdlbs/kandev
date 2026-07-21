"use client";

import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useDraftedIntegrationEnabled } from "./use-drafted-integration-enabled";

type Props = {
  id: string;
  enabled: boolean;
  persist: (enabled: boolean) => Promise<void> | void;
};

export function DraftedIntegrationEnabledControl({ id, enabled, persist }: Props) {
  const draft = useDraftedIntegrationEnabled({ id: `${id}-enabled`, enabled, persist });
  return (
    <div
      className="flex items-center gap-2 rounded-full border bg-muted/30 px-3 py-1"
      data-settings-dirty={draft.isDirty}
      data-settings-dirty-level="container"
    >
      <Switch
        id={`${id}-enabled`}
        checked={draft.enabled}
        data-settings-dirty={draft.isDirty}
        onCheckedChange={draft.setEnabled}
        className="cursor-pointer"
      />
      <Label htmlFor={`${id}-enabled`} className="text-xs cursor-pointer">
        {draft.enabled ? "Enabled" : "Disabled"}
      </Label>
    </div>
  );
}
