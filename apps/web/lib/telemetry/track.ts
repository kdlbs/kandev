// Frontend half of Kandev's strictly opt-in telemetry.
//
// Every helper here is fire-and-forget and consent-gated twice: locally
// against a cached copy of the consent record (so no request is even made
// without a grant), and again server-side where the backend enforces the
// event allowlist and drops anything sent without consent. Values must be
// short enum-like identifiers — the backend rejects free text.
import { fetchTelemetryConsent, sendTelemetryEvents } from "@/lib/api/domains/telemetry-api";
import type { TelemetryConsentState, TelemetryUIEventName } from "@/lib/types/telemetry";

let consentPromise: Promise<TelemetryConsentState | null> | null = null;

function loadConsent(): Promise<TelemetryConsentState | null> {
  if (consentPromise === null) {
    consentPromise = fetchTelemetryConsent().catch(() => null);
  }
  return consentPromise;
}

/**
 * Keep the local gate in sync when the settings card or onboarding step
 * changes consent, so tracking reacts immediately without a refetch.
 */
export function updateTelemetryConsentCache(state: TelemetryConsentState): void {
  consentPromise = Promise.resolve(state);
}

export function resetTelemetryTrackingForTests(): void {
  consentPromise = null;
}

async function track(name: TelemetryUIEventName, key: string, value: string): Promise<void> {
  try {
    const consent = await loadConsent();
    if (!consent || consent.env_disabled || consent.status !== "granted") return;
    await sendTelemetryEvents([{ name, properties: { [key]: value } }]);
  } catch {
    // Telemetry must never surface errors into the product.
  }
}

/**
 * Reduce a concrete pathname to an identifier-safe route label: IDs and
 * UUID-ish segments collapse to "id" so no user data rides along, and the
 * result fits the backend's ^[a-z0-9][a-z0-9_.:-]{0,63}$ allowlist.
 */
export function normalizePathForTelemetry(pathname: string): string {
  const segments = pathname
    .split("/")
    .filter(Boolean)
    .map((segment) => {
      const lowered = segment.toLowerCase();
      if (/^[0-9a-f]{8}-[0-9a-f-]{27,}$/.test(lowered) || /^\d+$/.test(lowered)) return "id";
      const cleaned = lowered.replace(/[^a-z0-9_-]/g, "");
      return cleaned === "" || /^[0-9a-f]{16,}$/.test(cleaned) ? "id" : cleaned;
    });
  if (segments.length === 0) return "home";
  return segments.join(".").slice(0, 64);
}

export function trackPageView(page: string): void {
  void track("ui_page_viewed", "page", page);
}

export function trackAction(action: string): void {
  void track("ui_action", "action", action);
}

export function trackFeatureUsed(feature: string): void {
  void track("feature_used", "feature", feature);
}
