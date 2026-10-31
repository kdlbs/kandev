"use client";

import { useEffect, useRef, useState } from "react";
import { acknowledgeNotificationSave } from "./notification-save-reconciliation";
import {
  createNotificationProvider,
  deleteNotificationProvider,
  testNotificationProvider,
  updateNotificationProvider,
} from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { t } from "@/lib/i18n";
import { DEFAULT_NOTIFICATION_EVENTS } from "@/lib/notifications/events";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import { sendTestNotification } from "./notification-test-action";
import { useNotificationProviders } from "@/hooks/domains/settings/use-notification-providers";
import { useAppStore } from "@/components/state-provider";
import {
  parseWebhookHeaders,
  useWebhookFormState,
  useWebhookProviderActions,
  webhookConfig,
  webhookHeadersError,
} from "./notifications-settings-webhook-helpers";
import {
  buildProviderUpdate,
  formatAppriseUrls,
  parseAppriseUrls,
} from "./notifications-settings-provider-updates";
import type { NotificationProvider } from "@/lib/types/http";

type AppriseFormMode = "create" | "edit";

type NotificationDraft = {
  providers: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  appriseEdits: Record<string, string>;
  appriseNameEdits: Record<string, string>;
  pendingDeletes: Set<string>;
  showAppriseForm: boolean;
  appriseFormMode: AppriseFormMode;
  showWebhookForm: boolean;
  webhookFormMode: AppriseFormMode;
  webhookHeaderDrafts: Record<string, string>;
};

type NotificationHydrationSource = {
  providers: NotificationProvider[] | undefined;
  events: string[] | undefined;
  appriseAvailable: boolean | undefined;
};

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

function hasDraftChanges({
  providers,
  baselineProviders,
  appriseEdits,
  appriseNameEdits,
  pendingDeletes,
  showAppriseForm,
  appriseFormMode,
  showWebhookForm,
  webhookFormMode,
  webhookHeaderDrafts,
}: NotificationDraft) {
  return (
    (showAppriseForm && appriseFormMode === "create") ||
    (showWebhookForm && webhookFormMode === "create") ||
    pendingDeletes.size > 0 ||
    Object.keys(webhookHeaderDrafts).some((providerId) => !pendingDeletes.has(providerId)) ||
    providers.some((provider) => {
      const baseline = baselineProviders.find((item) => item.id === provider.id);
      if (!baseline) return false;
      if (buildProviderUpdate(provider, baseline)) return true;
      if (provider.type !== "apprise") return false;
      const currentValue = appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls);
      const baselineValue = formatAppriseUrls(baseline.config?.urls);
      const nameValue = appriseNameEdits[provider.id] ?? provider.name;
      return currentValue !== baselineValue || nameValue !== baseline.name;
    })
  );
}

function isSameHydrationSource(
  current: NotificationHydrationSource | undefined,
  next: NotificationHydrationSource,
) {
  return (
    current?.providers === next.providers &&
    current?.events === next.events &&
    current?.appriseAvailable === next.appriseAvailable
  );
}

type NotificationDraftHydrationOptions = {
  loaded: boolean;
  source: NotificationHydrationSource;
  draft: NotificationDraft;
  setProviders: (value: NotificationProvider[]) => void;
  setBaselineProviders: (value: NotificationProvider[]) => void;
  setNotificationEvents: (value: string[]) => void;
  setAppriseEdits: (value: Record<string, string>) => void;
  setAppriseNameEdits: (value: Record<string, string>) => void;
};

function useHydrateNotificationDraft({
  loaded,
  source,
  draft,
  setProviders,
  setBaselineProviders,
  setNotificationEvents,
  setAppriseEdits,
  setAppriseNameEdits,
}: NotificationDraftHydrationOptions) {
  const hydratedSource = useRef<NotificationHydrationSource | undefined>(undefined);

  useEffect(() => {
    if (!loaded || isSameHydrationSource(hydratedSource.current, source)) return;
    if (hydratedSource.current && hasDraftChanges(draft)) return;

    hydratedSource.current = source;
    const nextProviders = source.providers ?? [];
    setProviders(nextProviders);
    setBaselineProviders(nextProviders);
    setNotificationEvents(source.events ?? []);
    const edits = buildAppriseEdits(nextProviders);
    setAppriseEdits(edits.urls);
    setAppriseNameEdits(edits.names);
  }, [
    draft.appriseEdits,
    draft.appriseFormMode,
    draft.appriseNameEdits,
    draft.baselineProviders,
    draft.pendingDeletes,
    draft.providers,
    draft.showAppriseForm,
    draft.showWebhookForm,
    draft.webhookFormMode,
    draft.webhookHeaderDrafts,
    loaded,
    setAppriseEdits,
    setAppriseNameEdits,
    setBaselineProviders,
    setNotificationEvents,
    setProviders,
    source.appriseAvailable,
    source.events,
    source.providers,
  ]);
}

export function useNotificationsState() {
  const {
    providers: storeProviders,
    events: storeEvents,
    appriseAvailable: storeAppriseAvailable,
    loaded,
    loading,
    rescanApprise,
    appriseRescanPending,
    appriseRescanError,
    appriseRescanResult,
  } = useNotificationProviders();
  const appriseAvailable = storeAppriseAvailable ?? false;
  const setNotificationProviders = useAppStore((state) => state.setNotificationProviders);
  const [providers, setProviders] = useState<NotificationProvider[]>(() => storeProviders ?? []);
  const [baselineProviders, setBaselineProviders] = useState<NotificationProvider[]>(
    () => storeProviders ?? [],
  );
  const [notificationEvents, setNotificationEvents] = useState<string[]>(() => storeEvents ?? []);
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
  const appriseFormIdentity = useRef<object>({});
  const appriseFormRef = useRef<{ open: boolean; mode: AppriseFormMode }>({
    open: false,
    mode: "create",
  });
  const [activeAppriseId, setActiveAppriseId] = useState<string | null>(null);
  const webhookForm = useWebhookFormState();
  const { showWebhookForm, webhookFormMode } = webhookForm;
  const [pendingDeletes, setPendingDeletes] = useState<Set<string>>(new Set());
  useHydrateNotificationDraft({
    loaded,
    source: {
      providers: storeProviders,
      events: storeEvents,
      appriseAvailable: storeAppriseAvailable,
    },
    draft: {
      providers,
      baselineProviders,
      appriseEdits,
      appriseNameEdits,
      pendingDeletes,
      showAppriseForm,
      appriseFormMode,
      showWebhookForm,
      webhookFormMode,
      webhookHeaderDrafts: webhookForm.webhookHeaderDrafts,
    },
    setProviders,
    setBaselineProviders,
    setNotificationEvents,
    setAppriseEdits,
    setAppriseNameEdits,
  });
  return {
    providers,
    setProviders,
    baselineProviders,
    setBaselineProviders,
    notificationEvents,
    setNotificationEvents,
    loaded,
    loading,
    appriseAvailable,
    rescanApprise,
    appriseRescanPending,
    appriseRescanError,
    appriseRescanResult,
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
    appriseFormIdentity,
    appriseFormRef,
    ...webhookForm,
    pendingDeletes,
    setPendingDeletes,
    setNotificationProviders,
  };
}

export type NotificationsState = ReturnType<typeof useNotificationsState>;
export type { AppriseFormMode };
export type PermissionRefresh = (error?: unknown) => void | Promise<void>;

type AppriseCreateDraft = { name: string; urls: string[] };
type WebhookCreateDraft = {
  name: string;
  url: string;
  headers: Record<string, string>;
  secret: string;
};

function buildAppriseCreateDraft(state: NotificationsState): AppriseCreateDraft | null {
  if (!state.showAppriseForm || state.appriseFormMode !== "create") return null;
  return { name: state.appriseName, urls: parseAppriseUrls(state.appriseUrls) };
}

function buildWebhookCreateDraft(state: NotificationsState): WebhookCreateDraft | null {
  if (!state.showWebhookForm || state.webhookFormMode !== "create") return null;
  return {
    name: state.webhookName,
    url: state.webhookUrl.trim(),
    headers: parseWebhookHeaders(state.webhookHeaders),
    secret: state.webhookSecret.trim(),
  };
}

async function createProviderFromDrafts(
  appriseDraft: AppriseCreateDraft | null,
  webhookDraft: WebhookCreateDraft | null,
): Promise<NotificationProvider | null> {
  if (appriseDraft) {
    return createNotificationProvider({
      name: appriseDraft.name.trim() || "Apprise",
      type: "apprise",
      config: { urls: appriseDraft.urls },
      enabled: true,
      events: DEFAULT_NOTIFICATION_EVENTS,
    });
  }
  if (webhookDraft) {
    return createNotificationProvider({
      name: webhookDraft.name.trim() || "Webhook",
      type: "webhook",
      config: {
        url: webhookDraft.url,
        headers: webhookDraft.headers,
        ...(webhookDraft.secret ? { secret: webhookDraft.secret } : {}),
      },
      enabled: true,
      events: DEFAULT_NOTIFICATION_EVENTS,
    });
  }
  return null;
}

function assertWebhookHeadersValid(state: NotificationsState, providers: NotificationProvider[]) {
  if (state.showWebhookForm && state.webhookFormMode === "create") {
    const headersError = webhookHeadersError(state.webhookHeaders);
    if (headersError) throw new Error(t(headersError));
  }
  for (const [providerId, rawHeaders] of Object.entries(state.webhookHeaderDrafts)) {
    if (state.pendingDeletes.has(providerId)) continue;
    const headersError = webhookHeadersError(rawHeaders);
    if (headersError) throw new Error(t(headersError));
  }
  for (const provider of providers) {
    if (provider.type !== "webhook") continue;
    const headersError = webhookHeadersError(webhookConfig(provider.config).headers);
    if (headersError) throw new Error(t(headersError));
  }
}

export function useSaveRequest(state: NotificationsState) {
  const currentState = useRef(state);
  currentState.current = state;
  const { providers, baselineProviders, pendingDeletes, setNotificationProviders } = state;
  return useRequest(async () => {
    const formIdentity = {
      apprise: state.appriseFormIdentity.current,
      webhook: state.webhookFormIdentity.current,
    };
    const appriseDraft = buildAppriseCreateDraft(state);
    if (appriseDraft && appriseDraft.urls.length === 0) {
      throw new Error(t("settings:appriseUrlRequired"));
    }
    const webhookDraft = buildWebhookCreateDraft(state);
    if (webhookDraft && !webhookDraft.url) {
      throw new Error(t("settings:webhookUrlRequired"));
    }
    assertWebhookHeadersValid(state, providers);
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
    const created = await createProviderFromDrafts(appriseDraft, webhookDraft);
    const updatedById = new Map(updated.map((provider) => [provider.id, provider]));
    const nextProviders = providers
      .map((provider) => updatedById.get(provider.id) ?? provider)
      .concat(created ? [created] : []);
    setNotificationProviders({
      items: nextProviders,
      events: state.notificationEvents,
      loaded: true,
      loading: false,
    });
    acknowledgeNotificationSave({
      submitted: state,
      current: currentState.current,
      accepted: nextProviders,
      edits: buildAppriseEdits(nextProviders),
      created,
      formIdentity,
    });
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
    showWebhookForm,
    webhookFormMode,
    webhookHeaderDrafts,
  } = state;
  return hasDraftChanges({
    providers,
    baselineProviders,
    appriseEdits,
    appriseNameEdits,
    pendingDeletes,
    showAppriseForm,
    appriseFormMode,
    showWebhookForm,
    webhookFormMode,
    webhookHeaderDrafts,
  });
}

function closeWebhookCreateDraft(state: NotificationsState) {
  state.webhookFormRef.current = { open: false, mode: "create" };
  state.webhookFormIdentity.current = {};
  state.setShowWebhookForm(false);
  state.setActiveWebhookId(null);
  state.setWebhookName("");
  state.setWebhookUrl("");
  state.setWebhookHeaders("");
  state.setWebhookSecret("");
}

function useAppriseProviderActions(state: NotificationsState) {
  const {
    baselineProviders,
    appriseEdits,
    appriseNameEdits,
    appriseFormMode,
    appriseFormRef,
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
    webhookFormRef,
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
    const webhookForm = webhookFormRef.current;
    if (mode === "create" && webhookForm.open && webhookForm.mode === "create") {
      closeWebhookCreateDraft(state);
    }
    state.appriseFormIdentity.current = {};
    appriseFormRef.current = { open: true, mode };
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
    state.appriseFormIdentity.current = {};
    appriseFormRef.current = { open: false, mode: "create" };
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

export function useNotificationsActions(
  state: NotificationsState,
  bumpPermission: PermissionRefresh,
) {
  const { setProviders } = state;
  const appriseActions = useAppriseProviderActions(state);
  const webhookActions = useWebhookProviderActions(state);

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
    try {
      if (nativeNotifications.isAvailable()) {
        const requests: Array<Promise<unknown>> = [nativeNotifications.permission.request()];
        if (typeof Notification !== "undefined") {
          requests.push(Notification.requestPermission());
        }
        await Promise.all(requests);
        await bumpPermission();
        return;
      }
      if (typeof Notification === "undefined") return;
      await Notification.requestPermission();
      await bumpPermission();
    } catch (error) {
      try {
        await bumpPermission(error);
      } catch {
        // A permission failure must never escape a user gesture handler.
      }
    }
  };

  const handleRefreshPermission = async () => {
    try {
      await bumpPermission();
    } catch {
      // A permission query failure is rendered by the permission state hook.
    }
  };

  const handleTestNotification = () => sendTestNotification(bumpPermission);

  const handleTestProvider = async (providerId: string) => {
    try {
      await testNotificationProvider(providerId);
    } catch (error) {
      console.error("[NotificationsSettings] Test notification failed", error);
    }
  };

  const discard = () => {
    state.appriseFormIdentity.current = {};
    state.webhookFormIdentity.current = {};
    state.appriseFormRef.current = { open: false, mode: "create" };
    state.webhookFormRef.current = { open: false, mode: "create" };
    const edits = buildAppriseEdits(state.baselineProviders);
    state.setProviders(state.baselineProviders);
    state.setAppriseEdits(edits.urls);
    state.setAppriseNameEdits(edits.names);
    state.setPendingDeletes(new Set());
    state.setAppriseName("");
    state.setAppriseUrls("");
    state.setShowAppriseForm(false);
    state.setActiveAppriseId(null);
    state.setShowWebhookForm(false);
    state.setActiveWebhookId(null);
    state.setWebhookName("");
    state.setWebhookUrl("");
    state.setWebhookHeaders("");
    state.setWebhookSecret("");
    state.setWebhookHeaderDrafts({});
  };

  return {
    handleToggleEvent,
    handleRequestPermission,
    handleRefreshPermission,
    handleTestNotification,
    handleTestProvider,
    discard,
    ...appriseActions,
    ...webhookActions,
  };
}
