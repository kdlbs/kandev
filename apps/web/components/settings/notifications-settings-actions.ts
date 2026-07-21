"use client";

import { useState } from "react";
import {
  createNotificationProvider,
  deleteNotificationProvider,
  testNotificationProvider,
  updateNotificationProvider,
} from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
import { useNotificationProviders } from "@/hooks/domains/settings/use-notification-providers";
import { useAppStore } from "@/components/state-provider";
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
  const setNotificationProviders = useAppStore((state) => state.setNotificationProviders);
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
    setNotificationProviders,
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
    setNotificationProviders,
    showAppriseForm,
    appriseFormMode,
    appriseName,
    setAppriseName,
    appriseUrls,
    setAppriseUrls,
    setShowAppriseForm,
    setActiveAppriseId,
  } = state;
  return useRequest(async () => {
    const createDraft =
      showAppriseForm && appriseFormMode === "create"
        ? { name: appriseName, urls: parseAppriseUrls(appriseUrls) }
        : null;
    if (createDraft && createDraft.urls.length === 0) {
      throw new Error("At least one Apprise service URL is required.");
    }
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
    const created = createDraft
      ? await createNotificationProvider({
          name: createDraft.name.trim() || "Apprise",
          type: "apprise",
          config: { urls: createDraft.urls },
          enabled: true,
          events:
            state.notificationEvents.length > 0
              ? state.notificationEvents
              : DEFAULT_NOTIFICATION_EVENTS,
        })
      : null;
    const updatedById = new Map(updated.map((provider) => [provider.id, provider]));
    const nextProviders = providers
      .map((provider) => updatedById.get(provider.id) ?? provider)
      .concat(created ? [created] : []);
    setNotificationProviders({
      items: nextProviders,
      events: state.notificationEvents,
      appriseAvailable: state.appriseAvailable,
      loaded: true,
      loading: false,
    });
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
    if (created) {
      setAppriseName("");
      setAppriseUrls("");
      setShowAppriseForm(false);
      setActiveAppriseId(null);
    }
    return created ? [...updated, created] : updated;
  });
}

export function useIsDirty(state: NotificationsState) {
  const {
    providers,
    baselineProviders,
    appriseEdits,
    appriseNameEdits,
    pendingDeletes,
    showAppriseForm,
    appriseFormMode,
  } = state;
  return (
    (showAppriseForm && appriseFormMode === "create") ||
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
    baselineProviders,
    appriseEdits,
    appriseNameEdits,
    appriseFormMode,
    activeAppriseId,
    setProviders,
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
    setAppriseName("");
    setAppriseUrls("");
  };

  const cancelAppriseForm = () => {
    if (appriseFormMode === "edit" && activeAppriseId) {
      const baseline = baselineProviders.find((provider) => provider.id === activeAppriseId);
      if (baseline) {
        setProviders((current) =>
          current.map((provider) =>
            provider.id === baseline.id
              ? {
                  ...provider,
                  name: baseline.name,
                  config: { ...provider.config, urls: baseline.config?.urls },
                }
              : provider,
          ),
        );
        setAppriseEdits((current) => ({
          ...current,
          [baseline.id]: formatAppriseUrls(baseline.config?.urls),
        }));
        setAppriseNameEdits((current) => ({ ...current, [baseline.id]: baseline.name }));
      }
    }
    closeAppriseForm();
  };

  return {
    handleAppriseEdit,
    handleAppriseNameEdit,
    openAppriseForm,
    handleDeleteProvider,
    closeAppriseForm,
    cancelAppriseForm,
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

  const handleTestProvider = async (providerId: string) => {
    try {
      await testNotificationProvider(providerId);
    } catch (error) {
      console.error("[NotificationsSettings] Test notification failed", error);
    }
  };

  const discard = () => {
    const edits = buildAppriseEdits(state.baselineProviders);
    state.setProviders(state.baselineProviders);
    state.setAppriseEdits(edits.urls);
    state.setAppriseNameEdits(edits.names);
    state.setPendingDeletes(new Set());
    state.setAppriseName("");
    state.setAppriseUrls("");
    state.setShowAppriseForm(false);
    state.setActiveAppriseId(null);
  };

  return {
    handleToggleEvent,
    handleRequestPermission,
    handleRefreshPermission,
    handleTestNotification,
    handleTestProvider,
    discard,
    ...appriseActions,
  };
}
