import type { NotificationProvider } from "@/lib/types/http";
import {
  parseWebhookHeaders,
  webhookConfig,
  webhookHeadersSignature,
} from "./notifications-settings-webhook-helpers";

type ProviderUpdatePayload = {
  enabled?: boolean;
  events?: string[];
  config?: NotificationProvider["config"];
  name?: string;
};

export function formatAppriseUrls(value: unknown): string {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === "string").join("\n");
  }
  if (typeof value === "string") {
    return value;
  }
  return "";
}

export function parseAppriseUrls(value: string): string[] {
  return value
    .split("\n")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function normalizeEvents(events?: string[]): string {
  if (!Array.isArray(events)) {
    return "";
  }
  return [...events].sort().join("|");
}

function extractUrls(config: NotificationProvider["config"]): string[] {
  if (!config) return [];
  const urls = config.urls;
  if (Array.isArray(urls)) {
    return urls.filter((item): item is string => typeof item === "string");
  }
  if (typeof urls === "string") {
    return parseAppriseUrls(urls);
  }
  return [];
}

export function buildProviderUpdate(
  provider: NotificationProvider,
  baseline: NotificationProvider,
): ProviderUpdatePayload | null {
  const updates: ProviderUpdatePayload = {};
  if (provider.enabled !== baseline.enabled) updates.enabled = provider.enabled;
  if (normalizeEvents(provider.events) !== normalizeEvents(baseline.events)) {
    updates.events = provider.events;
  }
  if (provider.name !== baseline.name) updates.name = provider.name;
  if (provider.type === "apprise") {
    const urls = extractUrls(provider.config);
    const baselineUrls = extractUrls(baseline.config);
    if (urls.join("|") !== baselineUrls.join("|")) {
      updates.config = { ...provider.config, urls };
    }
  }
  if (provider.type === "webhook") {
    const current = webhookConfig(provider.config);
    const previous = webhookConfig(baseline.config);
    if (
      current.url !== previous.url ||
      webhookHeadersSignature(current.headers) !== webhookHeadersSignature(previous.headers) ||
      current.secret !== previous.secret
    ) {
      updates.config = {
        url: current.url,
        headers: parseWebhookHeaders(current.headers),
        ...(current.secret ? { secret: current.secret } : {}),
      };
    }
  }
  return Object.keys(updates).length ? updates : null;
}
