/**
 * Client-side validation for the OpenAI-compatible provider section of the
 * agent profile editor. Mirrors the backend rules in
 * `internal/agent/settings/controller/profile_crud.go` +
 * `internal/common/acpprovider` so a save is blocked before the round-trip.
 *
 * Returns an i18n key (never rendered copy) so the caller owns translation and
 * `i18next/no-literal-string` stays satisfied in this plain `.ts` helper.
 */

export const PROVIDER_KIND_NATIVE = "";
export const PROVIDER_KIND_OPENAI_COMPATIBLE = "openai_compatible";

export type ProviderConfigDraft = {
  providerKind?: string;
  providerBaseUrl?: string;
  model?: string;
};

export function isOpenAICompatibleProvider(kind: string | undefined): boolean {
  return kind === PROVIDER_KIND_OPENAI_COMPATIBLE;
}

/** True when `raw` is a non-empty absolute http(s) URL with a host. */
export function isValidProviderBaseUrl(raw: string): boolean {
  const trimmed = raw.trim();
  if (trimmed === "") return false;
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return false;
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return false;
  return parsed.host !== "";
}

/**
 * Returns the i18n key for the first blocking problem with the provider
 * section, or undefined when the draft is savable. Native profiles are always
 * savable here.
 */
export function providerConfigInvalidReasonKey(draft: ProviderConfigDraft): string | undefined {
  if (!isOpenAICompatibleProvider(draft.providerKind)) return undefined;
  if (!isValidProviderBaseUrl(draft.providerBaseUrl ?? "")) {
    return "agents:providerBaseUrlInvalid";
  }
  if ((draft.model ?? "").includes("/")) {
    return "agents:providerModelSlash";
  }
  return undefined;
}

/**
 * Cleared provider fields for a profile switched back to native, so the saved
 * payload never carries a stale base URL or secret reference.
 */
export function clearedProviderFields(): {
  providerKind: string;
  providerBaseUrl: string;
  providerApiKeySecretId: string;
} {
  return { providerKind: "", providerBaseUrl: "", providerApiKeySecretId: "" };
}
