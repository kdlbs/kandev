"use client";

import { useState } from "react";
import {
  createNotificationProvider,
  deleteNotificationProvider,
  updateNotificationProvider,
} from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
import { useNotificationProviders } from "@/hooks/domains/settings/use-notification-providers";
import type { NotificationProvider } from "@/lib/types/http";

type ProviderUpdatePayload = {
  enabled?: boolean;
  events?: string[];
  config?: NotificationProvider["config"];
  name?: string;
};

type AppriseFormMode = "create" | "edit";

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
  return Object.keys(updates).length ? updates : null;
}

function buildAppriseEdits(providers: NotificationProvider[]) {
  const urls: Record<string, string> = {};
  const names: Record<string, string> = {};
  for (const provider of providers) {
    if (provider.type !== "apprise") continue;
    urls[provider.id] = formatAppriseUrls(provider.config?.urls);
    names[provider.id] = provider.name;
  }
  return { urls, names };
}

export function useNotificationsState() {
  const {
    providers: storeProviders,
    events: storeEvents,
    appriseAvailable: storeAppriseAvailable,
  } = useNotificationProviders();
  const [providers, setProviders] = useState<NotificationProvider[]>(() => storeProviders ?? []);
  const [baselineProviders, setBaselineProviders] = useState<NotificationProvider[]>(
    () => storeProviders ?? [],
  );
  const [notificationEvents] = useState<string[]>(() => storeEvents ?? []);
  const [appriseAvailable] = useState(() => storeAppriseAvailable ?? true);
  const [appriseName, setAppriseName] = useState("");
  const [appriseUrls, setAppriseUrls] = useState("");
  const [appriseEdits, setAppriseEdits] = useState<Record<string, string>>(
    () => buildAppriseEdits(storeProviders ?? []).urls,
  );
  const [appriseNameEdits, setAppriseNameEdits] = useState<Record<string, string>>(
    () => buildAppriseEdits(storeProviders ?? []).names,
  );
  const [showAppriseForm, setShowAppriseForm] = useState(false);
  const [appriseFormMode, setAppriseFormMode] = useState<AppriseFormMode>("create");
  const [activeAppriseId, setActiveAppriseId] = useState<string | null>(null);
  const [pendingDeletes, setPendingDeletes] = useState<Set<string>>(new Set());
  return {
    providers,
    setProviders,
    baselineProviders,
    setBaselineProviders,
    notificationEvents,
    appriseAvailable,
    appriseName,
    setAppriseName,
    appriseUrls,
    setAppriseUrls,
    appriseEdits,
    setAppriseEdits,
    appriseNameEdits,
    setAppriseNameEdits,
    showAppriseForm,
    setShowAppriseForm,
    appriseFormMode,
    setAppriseFormMode,
    activeAppriseId,
    setActiveAppriseId,
    pendingDeletes,
    setPendingDeletes,
  };
}

export type NotificationsState = ReturnType<typeof useNotificationsState>;
export type { AppriseFormMode };

export function useSaveRequest(state: NotificationsState) {
  const {
    providers,
    baselineProviders,
    setProviders,
    setBaselineProviders,
    setAppriseEdits,
    setAppriseNameEdits,
    pendingDeletes,
    setPendingDeletes,
  } = state;
  return useRequest(async () => {
    const updates: Array<Promise<NotificationProvider>> = [];
    for (const provider of providers) {
      const baseline = baselineProviders.find((item) => item.id === provider.id);
      if (!baseline) continue;
      const payload = buildProviderUpdate(provider, baseline);
      if (!payload) continue;
      updates.push(updateNotificationProvider(provider.id, payload));
    }
    for (const providerId of Array.from(pendingDeletes)) {
      await deleteNotificationProvider(providerId);
    }
    const updated = await Promise.all(updates);
    if (updated.length === 0) {
      setBaselineProviders(providers);
      setPendingDeletes(new Set());
      return [] as NotificationProvider[];
    }
    const updatedById = new Map(updated.map((provider) => [provider.id, provider]));
    const nextProviders = providers.map((provider) => updatedById.get(provider.id) ?? provider);
    setProviders(nextProviders);
    setBaselineProviders(nextProviders);
    setAppriseEdits((prev) => {
      const next = { ...prev };
      for (const p of nextProviders) {
        if (p.type === "apprise") next[p.id] = formatAppriseUrls(p.config?.urls);
      }
      return next;
    });
    setAppriseNameEdits((prev) => {
      const next = { ...prev };
      for (const p of nextProviders) {
        if (p.type === "apprise") next[p.id] = p.name;
      }
      return next;
    });
    setPendingDeletes(new Set());
    return updated;
  });
}

export function useIsDirty(state: NotificationsState) {
  const { providers, baselineProviders, appriseEdits, appriseNameEdits, pendingDeletes } = state;
  return (
    pendingDeletes.size > 0 ||
    providers.some((provider) => {
      const baseline = baselineProviders.find((item) => item.id === provider.id);
      if (!baseline) return false;
      if (buildProviderUpdate(provider, baseline)) return true;
      if (provider.type === "apprise") {
        const currentValue = appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls);
        const baselineValue = formatAppriseUrls(baseline.config?.urls);
        const nameValue = appriseNameEdits[provider.id] ?? provider.name;
        return currentValue !== baselineValue || nameValue !== baseline.name;
      }
      return false;
    })
  );
}

function useAppriseProviderActions(state: NotificationsState) {
  const {
    notificationEvents,
    appriseName,
    appriseUrls,
    appriseEdits,
    appriseNameEdits,
    setProviders,
    setBaselineProviders,
    setAppriseEdits,
    setAppriseNameEdits,
    setAppriseName,
    setAppriseUrls,
    setShowAppriseForm,
    setAppriseFormMode,
    setActiveAppriseId,
    setPendingDeletes,
  } = state;

  const updateProviderState = (
    providerId: string,
    updater: (p: NotificationProvider) => NotificationProvider,
  ) => {
    setProviders((prev) => prev.map((p) => (p.id === providerId ? updater(p) : p)));
  };

  const handleCreateAppriseProvider = async () => {
    const urls = parseAppriseUrls(appriseUrls);
    if (urls.length === 0) return;
    const defaultEvents =
      notificationEvents.length > 0 ? notificationEvents : DEFAULT_NOTIFICATION_EVENTS;
    try {
      const created = await createNotificationProvider({
        name: appriseName.trim() || "Apprise",
        type: "apprise",
        config: { urls },
        enabled: true,
        events: defaultEvents,
      });
      setProviders((prev) => [...prev, created]);
      setBaselineProviders((prev) => [...prev, created]);
      setAppriseEdits((prev) => ({ ...prev, [created.id]: urls.join("\n") }));
      setAppriseNameEdits((prev) => ({ ...prev, [created.id]: created.name }));
      setAppriseUrls("");
      setShowAppriseForm(false);
    } catch (error) {
      console.error("[NotificationsSettings] Failed to create apprise provider", error);
    }
  };

  const handleAppriseEdit = (providerId: string, value: string) => {
    setAppriseEdits((prev) => ({ ...prev, [providerId]: value }));
    updateProviderState(providerId, (p) => ({
      ...p,
      config: { ...p.config, urls: parseAppriseUrls(value) },
    }));
  };

  const handleAppriseNameEdit = (providerId: string, value: string) => {
    setAppriseNameEdits((prev) => ({ ...prev, [providerId]: value }));
    updateProviderState(providerId, (p) => ({ ...p, name: value }));
  };

  const openAppriseForm = (mode: AppriseFormMode, provider?: NotificationProvider) => {
    setAppriseFormMode(mode);
    setActiveAppriseId(provider?.id ?? null);
    if (provider) {
      setAppriseName(appriseNameEdits[provider.id] ?? provider.name);
      setAppriseUrls(appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls));
    } else {
      setAppriseName("");
      setAppriseUrls("");
    }
    setShowAppriseForm(true);
  };

  const handleDeleteProvider = (providerId: string) => {
    setPendingDeletes((prev) => new Set(prev).add(providerId));
    setProviders((prev) => prev.filter((p) => p.id !== providerId));
  };

  const closeAppriseForm = () => {
    setShowAppriseForm(false);
    setActiveAppriseId(null);
  };

  return {
    handleCreateAppriseProvider,
    handleAppriseEdit,
    handleAppriseNameEdit,
    openAppriseForm,
    handleDeleteProvider,
    closeAppriseForm,
    updateProviderState,
  };
}

export function useNotificationsActions(state: NotificationsState, bumpPermission: () => void) {
  const { setProviders } = state;
  const appriseActions = useAppriseProviderActions(state);

  const handleToggleEvent = (provider: NotificationProvider, eventType: string) => {
    const currentEvents = provider.events ?? [];
    const hasEvent = currentEvents.includes(eventType);
    const nextEvents = hasEvent
      ? currentEvents.filter((e) => e !== eventType)
      : [...currentEvents, eventType];
    setProviders((prev) =>
      prev.map((p) => (p.id === provider.id ? { ...p, events: nextEvents } : p)),
    );
  };

  const handleRequestPermission = async () => {
    if (typeof Notification === "undefined") return;
    await Notification.requestPermission();
    bumpPermission();
  };

  const handleRefreshPermission = () => {
    if (typeof Notification !== "undefined") bumpPermission();
  };

  const handleTestNotification = async () => {
    if (typeof Notification === "undefined") return;
    let permission = Notification.permission;
    if (permission === "default") {
      permission = await Notification.requestPermission();
      bumpPermission();
    }
    if (permission !== "granted") return;
    new Notification("Test notification", {
      body: "If you can read this, browser notifications are working.",
    });
  };

  return {
    handleToggleEvent,
    handleRequestPermission,
    handleRefreshPermission,
    handleTestNotification,
    ...appriseActions,
  };
}
