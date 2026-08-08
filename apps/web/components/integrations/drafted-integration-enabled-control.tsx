"use client";

import { Switch } from "@kandev/ui/switch";
import { useDraftedIntegrationEnabled } from "./use-drafted-integration-enabled";
import { useTranslation } from "react-i18next";

type Props = {
  id: string;
  enabled: boolean;
  persist: (enabled: boolean) => Promise<void> | void;
  /** Human-readable integration name (e.g. "GitHub") for the switch's accessible name. */
  name: string;
};

/**
 * Bare enable/disable slider for an integration — just the switch, no
 * visible "Enabled"/"Disabled" text — wired to the shared drafted-settings
 * (dirty/save) plumbing. Since there is no visible label to associate via
 * `htmlFor`, the accessible name comes from `aria-label`, built from `name`.
 */
export function DraftedIntegrationEnabledControl({ id, enabled, persist, name }: Props) {
  const { t } = useTranslation();
  const draft = useDraftedIntegrationEnabled({ id: `${id}-enabled`, enabled, persist });
  return (
    <Switch
      id={`${id}-enabled`}
      checked={draft.enabled}
      data-settings-dirty={draft.isDirty}
      data-settings-dirty-level="container"
      onCheckedChange={draft.setEnabled}
      aria-label={t("integrations:enableIntegrationNamed", { name })}
      className="cursor-pointer"
    />
  );
}
