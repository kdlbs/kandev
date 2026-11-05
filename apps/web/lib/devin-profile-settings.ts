/**
 * Pure Devin permission-intent helpers for the profile editor.
 *
 * Mode stays the single ACP authority and `DEVIN_SANDBOX` is an ordinary profile
 * environment entry. Mirror the backend defaults in
 * `internal/agent/settings/profileconfig/devin.go`; this module never rewrites
 * unrelated entries and never treats an inherited value as explicit disabled.
 */

export const DEVIN_AGENT_ID = "devin-acp";
export const DEVIN_SANDBOX_ENV_KEY = "DEVIN_SANDBOX";
export const DEVIN_SMART_MODE = "smart";
export const DEVIN_AUTONOMOUS_MODE = "autonomous";
export const DEVIN_BYPASS_MODE = "bypass";

export type DevinSandboxPreference = "inherit" | "disabled" | "enabled";

export type DevinEnvVar = {
  key: string;
  value?: string;
  secret_id?: string;
};

export type DevinPermissionProfile = {
  mode?: string;
  envVars?: DevinEnvVar[];
};

/** Reads the current override; custom and secret-backed values stay inherited. */
export function devinSandboxPreference(envVars: DevinEnvVar[] | undefined): DevinSandboxPreference {
  const entry = (envVars ?? []).find((envVar) => envVar.key === DEVIN_SANDBOX_ENV_KEY);
  if (!entry || entry.secret_id) return "inherit";
  if (entry.value === "true") return "enabled";
  if (entry.value === "false") return "disabled";
  return "inherit";
}

/** Whether the profile stores a custom (non-boolean) sandbox value. */
export function hasCustomDevinSandboxValue(envVars: DevinEnvVar[] | undefined): boolean {
  const entry = (envVars ?? []).find((envVar) => envVar.key === DEVIN_SANDBOX_ENV_KEY);
  if (!entry) return false;
  if (entry.secret_id) return true;
  return entry.value !== "true" && entry.value !== "false";
}

function replaceSandboxEntry(envVars: DevinEnvVar[], value: string | undefined): DevinEnvVar[] {
  const entries = [...(envVars ?? [])];
  const index = entries.findIndex((envVar) => envVar.key === DEVIN_SANDBOX_ENV_KEY);
  const replacement: DevinEnvVar = { key: DEVIN_SANDBOX_ENV_KEY, ...(value ? { value } : {}) };
  if (index === -1) {
    entries.push(replacement);
    return entries;
  }
  entries[index] = replacement;
  return entries;
}

function removeSandboxEntry(envVars: DevinEnvVar[] | undefined): DevinEnvVar[] {
  return (envVars ?? []).filter((envVar) => envVar.key !== DEVIN_SANDBOX_ENV_KEY);
}

/**
 * Applies one sandbox choice. Enabling requests Autonomous so an active
 * Smart/Bypass selection never contradicts the override; disabling or
 * inheriting keeps a non-Autonomous mode, or asks for a new one (Smart only
 * when the current discovery advertises it).
 */
export function devinSandboxPatch(
  profile: DevinPermissionProfile,
  preference: DevinSandboxPreference,
  options?: { smartAvailable?: boolean },
): { mode: string; envVars: DevinEnvVar[] } {
  const smartAvailable = options?.smartAvailable ?? true;
  const mode = profile.mode ?? "";
  if (preference === "enabled") {
    return {
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: replaceSandboxEntry(profile.envVars ?? [], "true"),
    };
  }
  if (preference === "inherit") {
    // "Use CLI settings" preserves the explicit mode and removes only the
    // override, so the provider's own policy applies.
    return { mode, envVars: removeSandboxEntry(profile.envVars) };
  }
  const envVars = replaceSandboxEntry(profile.envVars ?? [], "false");
  if (mode !== DEVIN_AUTONOMOUS_MODE) return { mode, envVars };
  return { mode: smartAvailable ? DEVIN_SMART_MODE : "", envVars };
}

/**
 * New-draft defaults for a Devin profile: Smart plus an explicit disabled
 * override. Saved profiles are projected through `toProfileFormData`, not this
 * helper, so their inherited values stay accurate.
 */
export function newDevinProfileDefaults(profile: DevinPermissionProfile): {
  mode: string;
  envVars: DevinEnvVar[];
} {
  const sandbox = (profile.envVars ?? []).find((envVar) => envVar.key === DEVIN_SANDBOX_ENV_KEY);
  const mode =
    (profile.mode ?? "").trim() ||
    (sandbox?.value === "true" ? DEVIN_AUTONOMOUS_MODE : DEVIN_SMART_MODE);
  if (sandbox) return { mode, envVars: profile.envVars ?? [] };
  return { mode, envVars: replaceSandboxEntry(profile.envVars ?? [], "false") };
}

/** Mirrors the backend rejection of an explicitly contradicting pair. */
export function devinPermissionConflict(
  mode: string | undefined,
  envVars: DevinEnvVar[] | undefined,
): boolean {
  const preference = devinSandboxPreference(envVars);
  if (preference === "inherit") return false;
  const normalized = (mode ?? "").trim().toLowerCase();
  if (preference === "enabled") {
    return normalized === DEVIN_SMART_MODE || normalized === DEVIN_BYPASS_MODE;
  }
  return normalized === DEVIN_AUTONOMOUS_MODE;
}
