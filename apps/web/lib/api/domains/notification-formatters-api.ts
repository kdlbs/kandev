import type { NotificationFormatterBinding, NotificationFormatterChoice } from "@kdlbs/plugin-sdk";
import { fetchJson, type ApiRequestOptions } from "../client";

export interface NotificationFormatterSettings {
  binding: NotificationFormatterBinding | null;
  formatters: NotificationFormatterChoice[];
  events: string[];
}

function formatterPath(providerId: string) {
  return `/api/v1/notification-providers/${encodeURIComponent(providerId)}/formatter`;
}

export function getNotificationFormatter(providerId: string, options?: ApiRequestOptions) {
  return fetchJson<NotificationFormatterSettings>(formatterPath(providerId), options);
}

export function selectNotificationFormatter(
  providerId: string,
  expectedRevision: string,
  binding: Omit<NotificationFormatterBinding, "provider_id" | "provider_type" | "revision"> | null,
) {
  return fetchJson<{ binding: NotificationFormatterBinding }>(formatterPath(providerId), {
    init: { method: "PUT", body: JSON.stringify({ expected_revision: expectedRevision, binding }) },
  });
}

export function previewNotificationFormatter(
  providerId: string,
  input: { binding: unknown; event: string; sample: string; draft: unknown },
  signal?: AbortSignal,
) {
  return fetchJson<{ title: string; body: string; used_fallback: boolean }>(
    `${formatterPath(providerId)}/preview`,
    {
      init: { method: "POST", body: JSON.stringify(input), signal },
    },
  );
}
