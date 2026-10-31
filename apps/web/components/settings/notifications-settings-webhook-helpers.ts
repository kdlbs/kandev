"use client";

import { useRef, useState } from "react";
import type { AppriseFormMode, NotificationsState } from "./notifications-settings-actions";
import type { NotificationProvider } from "@/lib/types/http";

const WEBHOOK_HEADERS_INVALID = "settings:webhookHeadersInvalid";
const WEBHOOK_HEADER_NAME = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/;
const INVALID_WEBHOOK_HEADER_VALUE = /[\u0000-\u0008\u000a-\u001f\u007f]/;

export function webhookHeadersError(value: string): string | null {
  const seen = new Set<string>();
  for (const line of value.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const separator = trimmed.indexOf(":");
    if (separator <= 0) return WEBHOOK_HEADERS_INVALID;
    const name = trimmed.slice(0, separator).trim();
    if (!name || !WEBHOOK_HEADER_NAME.test(name)) return WEBHOOK_HEADERS_INVALID;
    const key = name.toLowerCase();
    if (seen.has(key)) return WEBHOOK_HEADERS_INVALID;
    seen.add(key);
    const headerValue = trimmed.slice(separator + 1).trim();
    if (INVALID_WEBHOOK_HEADER_VALUE.test(headerValue)) {
      return WEBHOOK_HEADERS_INVALID;
    }
  }
  return null;
}

export function formatWebhookHeaders(value: unknown): string {
  if (!value || typeof value !== "object" || Array.isArray(value)) return "";
  return Object.entries(value as Record<string, unknown>)
    .filter((entry): entry is [string, string] => typeof entry[1] === "string")
    .map(([name, headerValue]) => `${name}: ${headerValue}`)
    .join("\n");
}

export function parseWebhookHeaders(value: string): Record<string, string> {
  const headers: Record<string, string> = {};
  for (const line of value.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const separator = trimmed.indexOf(":");
    if (separator <= 0) continue;
    const name = trimmed.slice(0, separator).trim();
    if (!name) continue;
    headers[name] = trimmed.slice(separator + 1).trim();
  }
  return headers;
}

export function webhookHeadersSignature(value: string): string {
  return Object.entries(parseWebhookHeaders(value))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, headerValue]) => `${name}: ${headerValue}`)
    .join("\n");
}

export function webhookConfig(config: NotificationProvider["config"] | undefined) {
  const source = config ?? {};
  const url = typeof source.url === "string" ? source.url : "";
  const secret = typeof source.secret === "string" ? source.secret : "";
  return { url, secret, headers: formatWebhookHeaders(source.headers) };
}

export function useWebhookFormState() {
  const [showWebhookForm, setShowWebhookForm] = useState(false);
  const [webhookFormMode, setWebhookFormMode] = useState<AppriseFormMode>("create");
  const [activeWebhookId, setActiveWebhookId] = useState<string | null>(null);
  const [webhookName, setWebhookName] = useState("");
  const [webhookUrl, setWebhookUrl] = useState("");
  const [webhookHeaders, setWebhookHeaders] = useState("");
  const [webhookHeaderDrafts, setWebhookHeaderDrafts] = useState<Record<string, string>>({});
  const [webhookSecret, setWebhookSecret] = useState("");
  const webhookFormIdentity = useRef<object>({});
  const webhookFormRef = useRef<{ open: boolean; mode: AppriseFormMode }>({
    open: false,
    mode: "create",
  });
  return {
    showWebhookForm,
    setShowWebhookForm,
    webhookFormMode,
    setWebhookFormMode,
    activeWebhookId,
    setActiveWebhookId,
    webhookName,
    setWebhookName,
    webhookUrl,
    setWebhookUrl,
    webhookHeaders,
    setWebhookHeaders,
    webhookHeaderDrafts,
    setWebhookHeaderDrafts,
    webhookSecret,
    setWebhookSecret,
    webhookFormIdentity,
    webhookFormRef,
  };
}

type WebhookProviderPatch = { url?: string; headers?: string; secret?: string };

function updateProvider(
  state: NotificationsState,
  providerId: string,
  updater: (provider: NotificationProvider) => NotificationProvider,
) {
  state.setProviders((prev) =>
    prev.map((provider) => (provider.id === providerId ? updater(provider) : provider)),
  );
}

function applyWebhookProviderEdit(
  state: NotificationsState,
  providerId: string,
  patch: WebhookProviderPatch,
) {
  if (patch.headers !== undefined) {
    const baseline = state.baselineProviders.find((provider) => provider.id === providerId);
    const baselineHeaders = webhookConfig(baseline?.config).headers;
    state.setWebhookHeaderDrafts((drafts) => {
      const next = { ...drafts };
      if (
        !webhookHeadersError(patch.headers!) &&
        webhookHeadersSignature(patch.headers!) === webhookHeadersSignature(baselineHeaders)
      ) {
        delete next[providerId];
      } else {
        next[providerId] = patch.headers!;
      }
      return next;
    });
  }
  updateProvider(state, providerId, (provider) => {
    const current = webhookConfig(provider.config);
    const url = patch.url ?? current.url;
    const rawHeaders = patch.headers ?? state.webhookHeaderDrafts[providerId] ?? current.headers;
    const headersError = webhookHeadersError(rawHeaders);
    const secret = patch.secret ?? current.secret;
    const config: NotificationProvider["config"] = { ...provider.config, url };
    if (!headersError) config.headers = parseWebhookHeaders(rawHeaders);
    if (secret) config.secret = secret;
    else delete config.secret;
    return { ...provider, config };
  });
}

export function useWebhookProviderActions(state: NotificationsState) {
  const {
    baselineProviders,
    appriseFormRef,
    appriseFormIdentity,
    webhookFormMode,
    webhookFormIdentity,
    webhookFormRef,
    activeWebhookId,
    setShowWebhookForm,
    setWebhookFormMode,
    setActiveWebhookId,
    setWebhookName,
    setWebhookUrl,
    webhookHeaders,
    setWebhookHeaders,
    webhookHeaderDrafts,
    setWebhookHeaderDrafts,
    setWebhookSecret,
    setShowAppriseForm,
    setActiveAppriseId,
    setAppriseName,
    setAppriseUrls,
  } = state;

  const handleWebhookEdit = (providerId: string, patch: WebhookProviderPatch) =>
    applyWebhookProviderEdit(state, providerId, patch);

  const handleWebhookNameEdit = (providerId: string, value: string) => {
    updateProvider(state, providerId, (provider) => ({ ...provider, name: value }));
  };

  const openWebhookForm = (mode: AppriseFormMode, provider?: NotificationProvider) => {
    const appriseForm = appriseFormRef.current;
    if (mode === "create" && appriseForm.open && appriseForm.mode === "create") {
      appriseFormRef.current = { open: false, mode: "create" };
      appriseFormIdentity.current = {};
      setShowAppriseForm(false);
      setActiveAppriseId(null);
      setAppriseName("");
      setAppriseUrls("");
    }
    webhookFormIdentity.current = {};
    webhookFormRef.current = { open: true, mode };
    setWebhookFormMode(mode);
    setActiveWebhookId(provider?.id ?? null);
    const config = webhookConfig(provider?.config);
    setWebhookName(provider?.name ?? "");
    setWebhookUrl(config.url);
    setWebhookHeaders(provider ? (webhookHeaderDrafts[provider.id] ?? config.headers) : "");
    setWebhookSecret(config.secret);
    setShowWebhookForm(true);
  };

  const clearWebhookForm = () => {
    webhookFormIdentity.current = {};
    webhookFormRef.current = { open: false, mode: "create" };
    setShowWebhookForm(false);
    setActiveWebhookId(null);
    setWebhookName("");
    setWebhookUrl("");
    setWebhookHeaders("");
    setWebhookSecret("");
  };

  const closeWebhookForm = () => {
    if (webhookHeadersError(webhookHeaders)) return;
    clearWebhookForm();
  };

  const cancelWebhookForm = () => {
    if (webhookFormMode === "edit" && activeWebhookId) {
      const baseline = baselineProviders.find((provider) => provider.id === activeWebhookId);
      if (baseline) {
        updateProvider(state, baseline.id, (provider) => ({
          ...provider,
          name: baseline.name,
          config: baseline.config,
        }));
      }
      setWebhookHeaderDrafts((drafts) => {
        const next = { ...drafts };
        delete next[activeWebhookId];
        return next;
      });
    }
    clearWebhookForm();
  };

  return {
    handleWebhookEdit,
    handleWebhookNameEdit,
    openWebhookForm,
    closeWebhookForm,
    cancelWebhookForm,
  };
}
