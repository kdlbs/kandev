'use client';

import { useMemo, useReducer, useState, useSyncExternalStore, type FormEvent } from 'react';
import { IconBell, IconRefresh } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@kandev/ui/hover-card';
import { Input } from '@kandev/ui/input';
import { Separator } from '@kandev/ui/separator';
import { Textarea } from '@kandev/ui/textarea';
import { SettingsPageTemplate } from '@/components/settings/settings-page-template';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import {
  createNotificationProvider,
  deleteNotificationProvider,
  updateNotificationProvider,
} from '@/lib/http';
import { useRequest } from '@/lib/http/use-request';
import { DEFAULT_NOTIFICATION_EVENTS, EVENT_LABELS } from '@/lib/notifications/events';
import { useNotificationProviders } from '@/hooks/use-notification-providers';
import type { NotificationProvider } from '@/lib/types/http';

type ProviderUpdatePayload = {
  enabled?: boolean;
  events?: string[];
  config?: NotificationProvider['config'];
  name?: string;
};

function formatAppriseUrls(value: unknown): string {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === 'string').join('\n');
  }
  if (typeof value === 'string') {
    return value;
  }
  return '';
}

function parseAppriseUrls(value: string): string[] {
  return value
    .split('\n')
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function extractUrls(config: NotificationProvider['config']): string[] {
  if (!config) {
    return [];
  }
  const urls = config.urls;
  if (Array.isArray(urls)) {
    return urls.filter((item): item is string => typeof item === 'string');
  }
  if (typeof urls === 'string') {
    return parseAppriseUrls(urls);
  }
  return [];
}

function normalizeEvents(events?: string[]): string {
  if (!Array.isArray(events)) {
    return '';
  }
  return [...events].sort().join('|');
}

type AppriseFormMode = 'create' | 'edit';

function buildAppriseEdits(providers: NotificationProvider[]) {
  const urls: Record<string, string> = {};
  const names: Record<string, string> = {};
  for (const provider of providers) {
    if (provider.type !== 'apprise') {
      continue;
    }
    urls[provider.id] = formatAppriseUrls(provider.config?.urls);
    names[provider.id] = provider.name;
  }
  return { urls, names };
}

export function NotificationsSettings() {
  const {
    providers: storeProviders,
    events: storeEvents,
    appriseAvailable: storeAppriseAvailable,
  } = useNotificationProviders();
  const [providers, setProviders] = useState<NotificationProvider[]>(() => storeProviders ?? []);
  const [baselineProviders, setBaselineProviders] = useState<NotificationProvider[]>(
    () => storeProviders ?? []
  );
  const [notificationEvents] = useState<string[]>(() => storeEvents ?? []);
  const [appriseAvailable] = useState(() => storeAppriseAvailable ?? true);
  const isProvidersLoading = false;
  const [appriseName, setAppriseName] = useState('');
  const [appriseUrls, setAppriseUrls] = useState('');
  const [appriseEdits, setAppriseEdits] = useState<Record<string, string>>(() => {
    return buildAppriseEdits(storeProviders ?? []).urls;
  });
  const [appriseNameEdits, setAppriseNameEdits] = useState<Record<string, string>>(() => {
    return buildAppriseEdits(storeProviders ?? []).names;
  });
  const [showAppriseForm, setShowAppriseForm] = useState(false);
  const [appriseFormMode, setAppriseFormMode] = useState<AppriseFormMode>('create');
  const [activeAppriseId, setActiveAppriseId] = useState<string | null>(null);
  const [pendingDeletes, setPendingDeletes] = useState<Set<string>>(new Set());
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false
  );
  const [, bumpPermission] = useReducer((value) => value + 1, 0);
  const notificationPermission: NotificationPermission | 'unsupported' = !mounted
    ? 'default'
    : typeof Notification === 'undefined'
      ? 'unsupported'
      : Notification.permission;

  const saveRequest = useRequest(async () => {
    const updates: Array<Promise<NotificationProvider>> = [];
    for (const provider of providers) {
      const baseline = baselineProviders.find((item) => item.id === provider.id);
      if (!baseline) {
        continue;
      }
      const payload = buildProviderUpdate(provider, baseline);
      if (!payload) {
        continue;
      }
      updates.push(updateNotificationProvider(provider.id, payload));
    }
    const deletions = Array.from(pendingDeletes);
    for (const providerId of deletions) {
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
      for (const provider of nextProviders) {
        if (provider.type === 'apprise') {
          next[provider.id] = formatAppriseUrls(provider.config?.urls);
        }
      }
      return next;
    });
    setAppriseNameEdits((prev) => {
      const next = { ...prev };
      for (const provider of nextProviders) {
        if (provider.type === 'apprise') {
          next[provider.id] = provider.name;
        }
      }
      return next;
    });
    setPendingDeletes(new Set());
    return updated;
  });

  // Store data is SSR-hydrated for settings. Local state is initialized once.

  const appriseProviders = providers.filter((provider) => provider.type === 'apprise');

  const tableProviders = useMemo(() => {
    return [...providers].sort((left, right) => {
      if (left.type === right.type) {
        return left.name.localeCompare(right.name);
      }
      if (left.type === 'local') {
        return -1;
      }
      if (right.type === 'local') {
        return 1;
      }
      return left.type.localeCompare(right.type);
    });
  }, [providers]);
  const tableEvents = useMemo(() => {
    if (notificationEvents.length > 0) {
      return notificationEvents;
    }
    const eventSet = new Set<string>();
    for (const provider of providers) {
      for (const event of provider.events ?? []) {
        eventSet.add(event);
      }
    }
    return eventSet.size ? Array.from(eventSet) : DEFAULT_NOTIFICATION_EVENTS;
  }, [notificationEvents, providers]);

  const isDirty =
    pendingDeletes.size > 0 ||
    providers.some((provider) => {
      const baseline = baselineProviders.find((item) => item.id === provider.id);
      if (!baseline) {
        return false;
      }
      if (buildProviderUpdate(provider, baseline)) {
        return true;
      }
      if (provider.type === 'apprise') {
        const currentValue = appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls);
        const baselineValue = formatAppriseUrls(baseline.config?.urls);
        const nameValue = appriseNameEdits[provider.id] ?? provider.name;
        return currentValue !== baselineValue || nameValue !== baseline.name;
      }
      return false;
    });

  const handleRequestPermission = async () => {
    if (typeof Notification === 'undefined') {
      return;
    }
    await Notification.requestPermission();
    bumpPermission();
  };

  const handleRefreshPermission = () => {
    if (typeof Notification === 'undefined') {
      return;
    }
    bumpPermission();
  };

  const handleTestNotification = async () => {
    if (typeof Notification === 'undefined') {
      return;
    }
    let permission = Notification.permission;
    if (permission === 'default') {
      permission = await Notification.requestPermission();
      bumpPermission();
    }
    if (permission !== 'granted') {
      return;
    }
    new Notification('Test notification', {
      body: 'If you can read this, browser notifications are working.',
    });
  };

  const updateProviderState = (providerId: string, updater: (provider: NotificationProvider) => NotificationProvider) => {
    setProviders((prev) => prev.map((provider) => (provider.id === providerId ? updater(provider) : provider)));
  };

  const handleToggleEvent = (provider: NotificationProvider, eventType: string) => {
    const currentEvents = provider.events ?? [];
    const hasEvent = currentEvents.includes(eventType);
    const nextEvents = hasEvent
      ? currentEvents.filter((event) => event !== eventType)
      : [...currentEvents, eventType];
    updateProviderState(provider.id, (current) => ({ ...current, events: nextEvents }));
  };

  const handleCreateAppriseProvider = async () => {
    const urls = parseAppriseUrls(appriseUrls);
    if (urls.length === 0) {
      return;
    }
    const defaultEvents =
      notificationEvents.length > 0 ? notificationEvents : DEFAULT_NOTIFICATION_EVENTS;
    try {
      const created = await createNotificationProvider({
        name: appriseName.trim() || 'Apprise',
        type: 'apprise',
        config: { urls },
        enabled: true,
        events: defaultEvents,
      });
      setProviders((prev) => [...prev, created]);
      setBaselineProviders((prev) => [...prev, created]);
      setAppriseEdits((prev) => ({ ...prev, [created.id]: urls.join('\n') }));
      setAppriseNameEdits((prev) => ({ ...prev, [created.id]: created.name }));
      setAppriseUrls('');
      setShowAppriseForm(false);
    } catch (error) {
      console.error('[NotificationsSettings] Failed to create apprise provider', error);
    }
  };

  const handleDeleteProvider = (providerId: string) => {
    setPendingDeletes((prev) => new Set(prev).add(providerId));
    setProviders((prev) => prev.filter((provider) => provider.id !== providerId));
  };

  const handleSave = async () => {
    try {
      await saveRequest.run();
    } catch (error) {
      console.error('[NotificationsSettings] Failed to save notifications', error);
    }
  };

  const handleAppriseEdit = (providerId: string, value: string) => {
    setAppriseEdits((prev) => ({ ...prev, [providerId]: value }));
    const urls = parseAppriseUrls(value);
    updateProviderState(providerId, (provider) => ({
      ...provider,
      config: { ...provider.config, urls },
    }));
  };

  const handleAppriseNameEdit = (providerId: string, value: string) => {
    setAppriseNameEdits((prev) => ({ ...prev, [providerId]: value }));
    updateProviderState(providerId, (provider) => ({
      ...provider,
      name: value,
    }));
  };

  const handleTextareaInput = (event: FormEvent<HTMLTextAreaElement>) => {
    const target = event.currentTarget;
    target.style.height = 'auto';
    target.style.height = `${target.scrollHeight}px`;
  };

  const openAppriseForm = (mode: AppriseFormMode, provider?: NotificationProvider) => {
    setAppriseFormMode(mode);
    setActiveAppriseId(provider?.id ?? null);
    if (provider) {
      setAppriseName(appriseNameEdits[provider.id] ?? provider.name);
      setAppriseUrls(appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls));
    } else {
      setAppriseName('');
      setAppriseUrls('');
    }
    setShowAppriseForm(true);
  };

  const closeAppriseForm = () => {
    setShowAppriseForm(false);
    setActiveAppriseId(null);
  };

  return (
    <SettingsPageTemplate
      title="Notifications"
      description="Configure providers and choose which events should alert you."
      isDirty={isDirty}
      saveStatus={saveRequest.status}
      onSave={handleSave}
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-base font-medium">Desktop Notifications</div>
            <p className="text-sm text-muted-foreground">
              Notify this device when an agent needs your input.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              title='Enable desktop notifications'
              variant="default"
              size="sm"
              onClick={handleRequestPermission}
              disabled={
                notificationPermission === 'granted' ||
                notificationPermission === 'unsupported'
              }
              className={
                notificationPermission === 'granted'
                  ? 'bg-emerald-500 text-white hover:bg-emerald-500'
                  : undefined
              }
            >
              {notificationPermission === 'granted' ? 'Enabled' : 'Enable'}
            </Button>
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon" onClick={handleRefreshPermission}>
                    <IconRefresh className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Refresh permission status</TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <HoverCard>
              <HoverCardTrigger asChild>
                <Button
                  title="Send test notification"
                  variant="outline"
                  className='cursor-pointer'
                  size="icon"
                  onClick={() => {
                    void handleTestNotification();
                  }}
                >
                  <IconBell className="h-4 w-4" />
                </Button>
              </HoverCardTrigger>
              <HoverCardContent side="top" className="text-sm">
                If you do not see notifications, check your OS settings and allow this browser.
              </HoverCardContent>
            </HoverCard>
          </div>
        </div>

        {notificationPermission === 'denied' && (
          <p className="text-sm text-amber-600">
            Notifications are blocked in your browser. Enable them in site settings, then click
            Refresh.
          </p>
        )}
        {notificationPermission === 'unsupported' && (
          <p className="text-sm text-amber-600">
            This browser does not support desktop notifications.
          </p>
        )}
      </div>

      <Separator className='my-4' />

      <div className="space-y-4">
        <div>
          <div className="text-base font-medium">External Providers</div>
          <p className="text-sm text-muted-foreground">
            Configure external providers for remote notifications.
          </p>
        </div>
        {!appriseAvailable && (
          <p className="text-sm text-muted-foreground">
            Apprise is not installed yet. You can add it later to enable remote notifications.{' '}
            <a
              className="underline"
              href="https://github.com/caronc/apprise?tab=readme-ov-file#installation"
              target="_blank"
              rel="noreferrer"
            >
              View installation instructions
            </a>
            .
          </p>
        )}
        {appriseProviders.length === 0 && (
          <p className="text-sm text-muted-foreground">No Apprise providers configured yet.</p>
        )}
        {appriseProviders.map((provider) => {
          const isEditing = appriseFormMode === 'edit' && activeAppriseId === provider.id;
          return (
            <div key={provider.id} className="rounded-lg border border-muted p-4 space-y-3">
              {isEditing ? (
                <AppriseProviderForm
                  mode="edit"
                  name={appriseName}
                  urls={appriseUrls}
                  onNameChange={(value) => {
                    setAppriseName(value);
                    handleAppriseNameEdit(provider.id, value);
                  }}
                  onUrlsChange={(value) => {
                    setAppriseUrls(value);
                    handleAppriseEdit(provider.id, value);
                  }}
                  onSubmit={async () => {
                    closeAppriseForm();
                  }}
                  onCancel={closeAppriseForm}
                  onInput={handleTextareaInput}
                />
              ) : (
                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-1 flex-1">
                    <div className="font-medium">{provider.name}</div>
                    <div className="text-xs text-muted-foreground">Apprise</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      className="cursor-pointer"
                      onClick={() => openAppriseForm('edit', provider)}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="cursor-pointer"
                      onClick={() => handleDeleteProvider(provider.id)}
                    >
                      Remove
                    </Button>
                  </div>
                </div>
              )}
            </div>
          );
        })}
        {appriseAvailable && (
          <div className="space-y-3">
            <Button
              variant="outline"
              className="cursor-pointer"
              onClick={() => openAppriseForm('create')}
            >
              Add Apprise Provider
            </Button>
            {showAppriseForm && appriseFormMode === 'create' && (
              <AppriseProviderForm
                mode="create"
                name={appriseName}
                urls={appriseUrls}
                onNameChange={(value) => setAppriseName(value)}
                onUrlsChange={(value) => setAppriseUrls(value)}
                onSubmit={async () => {
                  await handleCreateAppriseProvider();
                  closeAppriseForm();
                }}
                onCancel={closeAppriseForm}
                onInput={handleTextareaInput}
              />
            )}
          </div>
        )}
      </div>

      <Separator className='my-4' />

      <div className="space-y-4">
        <div>
          <div className="text-base font-medium">Notification Events</div>
          <p className="text-sm text-muted-foreground">
            Select which providers should receive each notification type.
          </p>
        </div>
        {isProvidersLoading && (
          <p className="text-sm text-muted-foreground">Loading providers...</p>
        )}
        {!isProvidersLoading && tableProviders.length === 0 && (
          <p className="text-sm text-muted-foreground">No providers configured yet.</p>
        )}
        {tableProviders.length > 0 && (
          <div className="overflow-auto rounded-lg border border-muted">
            <table className="min-w-full text-sm">
              <thead className="bg-muted/40">
                <tr>
                  <th className="px-4 py-3 text-left font-medium">Notification type</th>
                  {tableProviders.map((provider) => (
                    <th key={provider.id} className="px-4 py-3 text-center font-medium">
                      {provider.name}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {tableEvents.map((eventType) => {
                  const meta = EVENT_LABELS[eventType] ?? {
                    title: eventType,
                    description: 'Notify when this event occurs.',
                  };
                  return (
                    <tr key={eventType} className="border-t border-muted">
                      <td className="px-4 py-3">
                        <div className="font-medium">{meta.title}</div>
                        <div className="text-xs text-muted-foreground">{meta.description}</div>
                      </td>
                      {tableProviders.map((provider) => (
                        <td key={provider.id} className="px-4 py-3 text-center">
                          <div className="flex justify-center">
                            <Checkbox
                              checked={(provider.events ?? []).includes(eventType)}
                              onCheckedChange={() => handleToggleEvent(provider, eventType)}
                            />
                          </div>
                        </td>
                      ))}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </SettingsPageTemplate>
  );
}

type AppriseProviderFormProps = {
  mode: AppriseFormMode;
  name: string;
  urls: string;
  onNameChange: (value: string) => void;
  onUrlsChange: (value: string) => void;
  onSubmit: () => void | Promise<void>;
  onCancel: () => void;
  onInput: (event: FormEvent<HTMLTextAreaElement>) => void;
};

function AppriseProviderForm({
  mode,
  name,
  urls,
  onNameChange,
  onUrlsChange,
  onSubmit,
  onCancel,
  onInput,
}: AppriseProviderFormProps) {
  return (
    <div className="rounded-lg border border-dashed border-muted p-4 space-y-3">
      <div className="text-base font-medium">Apprise Provider</div>
      <Input
        value={name}
        onChange={(event) => onNameChange(event.target.value)}
        placeholder="Provider name"
      />
      <Textarea
        value={urls}
        onChange={(event) => onUrlsChange(event.target.value)}
        onInput={onInput}
        placeholder="Service URL(s)"
        rows={1}
        className="min-h-0 h-auto"
      />
      <div className="flex items-center gap-2">
        <Button className="cursor-pointer" onClick={onSubmit}>
          {mode === 'create' ? 'Add provider' : 'Done'}
        </Button>
        <Button variant="ghost" className="cursor-pointer" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

function buildProviderUpdate(
  provider: NotificationProvider,
  baseline: NotificationProvider
): ProviderUpdatePayload | null {
  const updates: ProviderUpdatePayload = {};
  if (provider.enabled !== baseline.enabled) {
    updates.enabled = provider.enabled;
  }
  if (normalizeEvents(provider.events) !== normalizeEvents(baseline.events)) {
    updates.events = provider.events;
  }
  if (provider.name !== baseline.name) {
    updates.name = provider.name;
  }
  if (provider.type === 'apprise') {
    const urls = extractUrls(provider.config);
    const baselineUrls = extractUrls(baseline.config);
    if (urls.join('|') !== baselineUrls.join('|')) {
      updates.config = { ...provider.config, urls };
    }
  }
  return Object.keys(updates).length ? updates : null;
}
