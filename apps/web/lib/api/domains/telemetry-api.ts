import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  TelemetryConsentState,
  TelemetryConsentStatus,
  TelemetryEventsResponse,
  TelemetryUIEvent,
} from "@/lib/types/telemetry";

const TELEMETRY_BASE = "/api/v1/telemetry";

export function fetchTelemetryConsent(options?: ApiRequestOptions): Promise<TelemetryConsentState> {
  return fetchJson<TelemetryConsentState>(`${TELEMETRY_BASE}/consent`, {
    ...options,
    cache: "no-store",
  });
}

export function updateTelemetryConsent(
  status: Exclude<TelemetryConsentStatus, "unasked">,
  options?: ApiRequestOptions,
): Promise<TelemetryConsentState> {
  return fetchJson<TelemetryConsentState>(`${TELEMETRY_BASE}/consent`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PUT",
      body: JSON.stringify({ status }),
    },
  });
}

export function sendTelemetryEvents(
  events: TelemetryUIEvent[],
  options?: ApiRequestOptions,
): Promise<TelemetryEventsResponse> {
  return fetchJson<TelemetryEventsResponse>(`${TELEMETRY_BASE}/events`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({ events }),
    },
  });
}
