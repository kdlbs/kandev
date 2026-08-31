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
  providerApiKeySecretId?: string;
  model?: string;
};

const LOOPBACK_HOSTNAMES = new Set(["localhost", "127.0.0.1", "[::1]", "::1"]);

function isLoopbackHostname(hostname: string): boolean {
  if (LOOPBACK_HOSTNAMES.has(hostname)) return true;
  return /^127(?:\.\d{1,3}){3}$/.test(hostname);
}

/**
 * True when `raw` is safe to send a bearer credential to: https, or http only
 * to a loopback host. Mirrors `acpprovider.ValidateCredentialedBaseURL` so the
 * editor blocks the same cleartext-credential leak the backend rejects.
 */
export function isCredentialTransportSafe(raw: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(raw.trim());
  } catch {
    return false;
  }
  if (parsed.protocol === "https:") return true;
  return parsed.protocol === "http:" && isLoopbackHostname(parsed.hostname);
}

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
  const baseUrl = draft.providerBaseUrl ?? "";
  if (!isValidProviderBaseUrl(baseUrl)) {
    return "agents:providerBaseUrlInvalid";
  }
  if ((draft.providerApiKeySecretId ?? "").trim() !== "" && !isCredentialTransportSafe(baseUrl)) {
    // Reuses the base-URL-invalid key (no new catalog entry): the precise
    // "use https for a credentialed provider" reason comes from the backend on
    // save. The point here is to block the cleartext-credential save up front.
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
