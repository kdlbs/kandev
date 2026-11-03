"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";

import { SettingsRow } from "@/components/settings/settings-group";
import { settingsControlClassName } from "@/components/settings/settings-control";
import {
  DEVIN_AGENT_ID,
  DEVIN_SMART_MODE,
  devinSandboxPatch,
  devinSandboxPreference,
  hasCustomDevinSandboxValue,
  type DevinEnvVar,
  type DevinSandboxPreference,
} from "@/lib/devin-profile-settings";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";

const PREFERENCE_OPTIONS: DevinSandboxPreference[] = ["inherit", "disabled", "enabled"];

export type DevinSandboxPreferenceFieldProps = {
  agentName: string;
  /** Profile-form shape: `env_vars` entries, not the camelCase draft. */
  profile: { mode: string; env_vars?: DevinEnvVar[] };
  baselineProfile?: { mode: string; env_vars?: DevinEnvVar[] };
  onChange: (patch: {
    mode: string;
    env_vars: DevinEnvVar[];
    devin_sandbox_inherited: boolean;
  }) => void;
  smartAvailable: boolean;
  /** Only report an unavailable Smart mode after discovery has settled. */
  discoveryReady?: boolean;
};

/** Whether this agent exposes the Devin sandbox override at all. */
export function supportsDevinSandbox(agentName: string): boolean {
  return agentName === DEVIN_AGENT_ID;
}

/**
 * Three-choice sandbox override. Absence ("Use CLI settings") and explicit
 * disabled are different provider states, so a boolean switch cannot represent
 * both. Enabling requests Autonomous through the same mode authority as the
 * Start mode selector.
 */
export function DevinSandboxPreferenceField({
  agentName,
  profile,
  baselineProfile,
  onChange,
  smartAvailable,
  discoveryReady = true,
}: DevinSandboxPreferenceFieldProps) {
  const { t } = useTranslation();
  const instanceId = useId();
  const descriptionId = `${instanceId}-devin-sandbox-description`;
  if (!supportsDevinSandbox(agentName)) return null;
  const preference = devinSandboxPreference(profile.env_vars);
  const baselinePreference = baselineProfile
    ? devinSandboxPreference(baselineProfile.env_vars)
    : undefined;
  const custom = hasCustomDevinSandboxValue(profile.env_vars);
  const smartUnavailable =
    discoveryReady && (profile.mode ?? "") === DEVIN_SMART_MODE && !smartAvailable;
  const isDirty =
    baselinePreference !== undefined &&
    (preference !== baselinePreference || (profile.mode ?? "") !== (baselineProfile?.mode ?? ""));

  return (
    <div className="space-y-2" data-testid="devin-sandbox-preference">
      <SettingsRow
        label={t("agents:devinSandboxLabel")}
        description={t("agents:devinSandboxDescription")}
        descriptionId={descriptionId}
        controlId={`${instanceId}-devin-sandbox`}
        isDirty={isDirty}
        control={
          <Select
            value={preference}
            onValueChange={(value) => {
              const next = PREFERENCE_OPTIONS.find((option) => option === value);
              if (!next) return;
              const patch = devinSandboxPatch(
                { mode: profile.mode, envVars: profile.env_vars },
                next,
                { smartAvailable },
              );
              onChange({
                mode: patch.mode,
                env_vars: patch.envVars,
                devin_sandbox_inherited: next === "inherit",
              });
            }}
          >
            <SelectTrigger
              id={`${instanceId}-devin-sandbox`}
              aria-describedby={descriptionId}
              className={settingsControlClassName("w-full cursor-pointer")}
              data-testid="devin-sandbox-select"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inherit" className="cursor-pointer">
                {t("agents:devinSandboxInherit")}
              </SelectItem>
              <SelectItem value="disabled" className="cursor-pointer">
                {t("agents:devinSandboxDisabled")}
              </SelectItem>
              <SelectItem value="enabled" className="cursor-pointer">
                {t("agents:devinSandboxEnabled")}
              </SelectItem>
            </SelectContent>
          </Select>
        }
      />
      {custom && (
        <p className="px-1 text-xs text-muted-foreground" data-testid="devin-sandbox-custom">
          {t("agents:devinSandboxCustom")}
        </p>
      )}
      {preference === "enabled" && (
        <p className="px-1 text-xs text-muted-foreground" data-testid="devin-sandbox-autonomous">
          {t("agents:devinSandboxAutonomousNote")}
        </p>
      )}
      {smartUnavailable && (
        <p
          className="px-1 text-xs text-destructive"
          role="alert"
          data-testid="devin-smart-unavailable"
        >
          {t("agents:devinSmartUnavailable")}
        </p>
      )}
    </div>
  );
}
