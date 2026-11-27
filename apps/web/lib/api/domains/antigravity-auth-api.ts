import { fetchJson, type ApiRequestOptions } from "../client";

export type AntigravityLoginState =
  | "starting"
  | "waiting_for_browser"
  | "checking"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "expired";

export type AntigravityLoginSnapshot = {
  attempt_id: string;
  state: AntigravityLoginState;
  expires_at: string;
  authorization_url?: string;
  error_code?: string;
  error?: string;
};

const BASE = "/api/v1/antigravity-acp/login";

export async function startAntigravityLogin(
  options?: ApiRequestOptions,
): Promise<AntigravityLoginSnapshot> {
  return fetchJson<AntigravityLoginSnapshot>(BASE, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function getAntigravityLogin(
  attemptId?: string,
  options?: ApiRequestOptions,
): Promise<AntigravityLoginSnapshot> {
  const path = attemptId ? `${BASE}/${encodeURIComponent(attemptId)}` : BASE;
  return fetchJson<AntigravityLoginSnapshot>(path, options);
}

export async function cancelAntigravityLogin(
  attemptId: string,
  options?: ApiRequestOptions,
): Promise<AntigravityLoginSnapshot> {
  return fetchJson<AntigravityLoginSnapshot>(`${BASE}/${encodeURIComponent(attemptId)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}
