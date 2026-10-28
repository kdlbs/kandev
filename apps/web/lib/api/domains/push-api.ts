import { fetchJson, type ApiRequestOptions } from "../client";

export type PushSubscriptionRecord = {
  endpoint: string;
  user_agent: string;
  created_at: string;
  last_used_at?: string;
};

export type PushSubscriptionPayload = {
  endpoint: string;
  keys: { p256dh: string; auth: string };
};

export async function getPushPublicKey(options?: ApiRequestOptions) {
  return fetchJson<{ public_key: string }>("/api/v1/push/public-key", {
    ...options,
    cache: "no-store",
  });
}

export async function listPushSubscriptions(options?: ApiRequestOptions) {
  return fetchJson<{ subscriptions: PushSubscriptionRecord[] }>("/api/v1/push/subscriptions", {
    ...options,
    cache: "no-store",
  });
}

export async function savePushSubscription(
  payload: PushSubscriptionPayload,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>("/api/v1/push/subscriptions", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deletePushSubscription(endpoint: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>("/api/v1/push/subscriptions", {
    ...options,
    init: { method: "DELETE", body: JSON.stringify({ endpoint }), ...(options?.init ?? {}) },
  });
}

export async function sendTestPush(options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean; delivered: number }>("/api/v1/push/test", {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
