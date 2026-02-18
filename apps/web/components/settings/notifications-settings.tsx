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
} from '@/lib/api';
import { useRequest } from '@/lib/http/use-request';
import { DEFAULT_NOTIFICATION_EVENTS, EVENT_LABELS } from '@/lib/notifications/events';
import { useNotificationProviders } from '@/hooks/domains/settings/use-notification-providers';
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

type DesktopNotificationsSectionProps = {
  notificationPermission: NotificationPermission | 'unsupported';
  onRequestPermission: () => void;
  onRefreshPermission: () => void;
  onTestNotification: () => void;
};

function DesktopNotificationsSection({
  notificationPermission,
  onRequestPermission,
  onRefreshPermission,
  onTestNotification,
}: DesktopNotificationsSectionProps) {
  return (
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
            onClick={onRequestPermission}
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
                <Button variant="ghost" size="icon" onClick={onRefreshPermission}>
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
                  void onTestNotification();
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
  );
}

type NotificationEventsTableProps = {
  tableProviders: NotificationProvider[];
  tableEvents: string[];
  onToggleEvent: (provider: NotificationProvider, eventType: string) => void;
};

function NotificationEventsTable({
  tableProviders,
  tableEvents,
  onToggleEvent,
}: NotificationEventsTableProps) {
  if (tableProviders.length === 0) {
    return <p className="text-sm text-muted-foreground">No providers configured yet.</p>;
  }

  return (
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
                        onCheckedChange={() => onToggleEvent(provider, eventType)}
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
  );
}

/** Renders the list of Apprise providers with edit/remove capabilities. */
function AppriseProviderList({
  providers,
  appriseFormMode,
  activeAppriseId,
  appriseName,
  appriseUrls,
  onNameChange,
  onUrlsChange,
  onAppriseNameEdit,
  onAppriseEdit,
  onOpenForm,
  onCloseForm,
  onDeleteProvider,
  onTextareaInput,
}: {
  providers: NotificationProvider[];
  appriseFormMode: AppriseFormMode;
  activeAppriseId: string | null;
  appriseName: string;
  appriseUrls: string;
  onNameChange: (value: string) => void;
  onUrlsChange: (value: string) => void;
  onAppriseNameEdit: (providerId: string, value: string) => void;
  onAppriseEdit: (providerId: string, value: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onDeleteProvider: (providerId: string) => void;
  onTextareaInput: (event: FormEvent<HTMLTextAreaElement>) => void;
}) {
  return (
    <>
      {providers.map((provider) => {
        const isEditing = appriseFormMode === 'edit' && activeAppriseId === provider.id;
        return (
          <div key={provider.id} className="rounded-lg border border-muted p-4 space-y-3">
            {isEditing ? (
              <AppriseProviderForm
                mode="edit"
                name={appriseName}
                urls={appriseUrls}
                onNameChange={(value) => { onNameChange(value); onAppriseNameEdit(provider.id, value); }}
                onUrlsChange={(value) => { onUrlsChange(value); onAppriseEdit(provider.id, value); }}
                onSubmit={onCloseForm}
                onCancel={onCloseForm}
                onInput={onTextareaInput}
              />
            ) : (
              <div className="flex items-center justify-between gap-4">
                <div className="space-y-1 flex-1">
                  <div className="font-medium">{provider.name}</div>
                  <div className="text-xs text-muted-foreground">Apprise</div>
                </div>
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" className="cursor-pointer" onClick={() => onOpenForm('edit', provider)}>Edit</Button>
                  <Button variant="outline" size="sm" className="cursor-pointer" onClick={() => onDeleteProvider(provider.id)}>Remove</Button>
                </div>
              </div>
            )}
          </div>
        );
      })}
    </>
  );
}

function useNotificationPermission() {
  const mounted = useSyncExternalStore(() => () => undefined, () => true, () => false);
  const [, bumpPermission] = useReducer((value) => value + 1, 0);
  let notificationPermission: NotificationPermission | 'unsupported';
  if (!mounted) notificationPermission = 'default';
  else if (typeof Notification === 'undefined') notificationPermission = 'unsupported';
  else notificationPermission = Notification.permission;
  return { notificationPermission, bumpPermission };
}

function useNotificationsState() {
  const { providers: storeProviders, events: storeEvents, appriseAvailable: storeAppriseAvailable } = useNotificationProviders();
  const [providers, setProviders] = useState<NotificationProvider[]>(() => storeProviders ?? []);
  const [baselineProviders, setBaselineProviders] = useState<NotificationProvider[]>(() => storeProviders ?? []);
  const [notificationEvents] = useState<string[]>(() => storeEvents ?? []);
  const [appriseAvailable] = useState(() => storeAppriseAvailable ?? true);
  const [appriseName, setAppriseName] = useState('');
  const [appriseUrls, setAppriseUrls] = useState('');
  const [appriseEdits, setAppriseEdits] = useState<Record<string, string>>(() => buildAppriseEdits(storeProviders ?? []).urls);
  const [appriseNameEdits, setAppriseNameEdits] = useState<Record<string, string>>(() => buildAppriseEdits(storeProviders ?? []).names);
  const [showAppriseForm, setShowAppriseForm] = useState(false);
  const [appriseFormMode, setAppriseFormMode] = useState<AppriseFormMode>('create');
  const [activeAppriseId, setActiveAppriseId] = useState<string | null>(null);
  const [pendingDeletes, setPendingDeletes] = useState<Set<string>>(new Set());
  return {
    providers, setProviders, baselineProviders, setBaselineProviders,
    notificationEvents, appriseAvailable,
    appriseName, setAppriseName, appriseUrls, setAppriseUrls,
    appriseEdits, setAppriseEdits, appriseNameEdits, setAppriseNameEdits,
    showAppriseForm, setShowAppriseForm, appriseFormMode, setAppriseFormMode,
    activeAppriseId, setActiveAppriseId, pendingDeletes, setPendingDeletes,
  };
}

type NotificationsState = ReturnType<typeof useNotificationsState>;

function useSaveRequest(state: NotificationsState) {
  const { providers, baselineProviders, setProviders, setBaselineProviders, setAppriseEdits, setAppriseNameEdits, pendingDeletes, setPendingDeletes } = state;
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
    if (updated.length === 0) { setBaselineProviders(providers); setPendingDeletes(new Set()); return [] as NotificationProvider[]; }
    const updatedById = new Map(updated.map((provider) => [provider.id, provider]));
    const nextProviders = providers.map((provider) => updatedById.get(provider.id) ?? provider);
    setProviders(nextProviders);
    setBaselineProviders(nextProviders);
    setAppriseEdits((prev) => { const next = { ...prev }; for (const p of nextProviders) { if (p.type === 'apprise') next[p.id] = formatAppriseUrls(p.config?.urls); } return next; });
    setAppriseNameEdits((prev) => { const next = { ...prev }; for (const p of nextProviders) { if (p.type === 'apprise') next[p.id] = p.name; } return next; });
    setPendingDeletes(new Set());
    return updated;
  });
}

function useNotificationsActions(state: NotificationsState, bumpPermission: () => void) {
  const { notificationEvents, appriseName, appriseUrls, appriseEdits, appriseNameEdits, setProviders, setBaselineProviders, setAppriseEdits, setAppriseNameEdits, setAppriseName, setAppriseUrls, setShowAppriseForm, setAppriseFormMode, setActiveAppriseId, setPendingDeletes } = state;

  const updateProviderState = (providerId: string, updater: (p: NotificationProvider) => NotificationProvider) => {
    setProviders((prev) => prev.map((p) => (p.id === providerId ? updater(p) : p)));
  };

  const handleToggleEvent = (provider: NotificationProvider, eventType: string) => {
    const currentEvents = provider.events ?? [];
    const hasEvent = currentEvents.includes(eventType);
    const nextEvents = hasEvent ? currentEvents.filter((e) => e !== eventType) : [...currentEvents, eventType];
    updateProviderState(provider.id, (current) => ({ ...current, events: nextEvents }));
  };

  const handleCreateAppriseProvider = async () => {
    const urls = parseAppriseUrls(appriseUrls);
    if (urls.length === 0) return;
    const defaultEvents = notificationEvents.length > 0 ? notificationEvents : DEFAULT_NOTIFICATION_EVENTS;
    try {
      const created = await createNotificationProvider({ name: appriseName.trim() || 'Apprise', type: 'apprise', config: { urls }, enabled: true, events: defaultEvents });
      setProviders((prev) => [...prev, created]);
      setBaselineProviders((prev) => [...prev, created]);
      setAppriseEdits((prev) => ({ ...prev, [created.id]: urls.join('\n') }));
      setAppriseNameEdits((prev) => ({ ...prev, [created.id]: created.name }));
      setAppriseUrls('');
      setShowAppriseForm(false);
    } catch (error) { console.error('[NotificationsSettings] Failed to create apprise provider', error); }
  };

  const handleDeleteProvider = (providerId: string) => {
    setPendingDeletes((prev) => new Set(prev).add(providerId));
    setProviders((prev) => prev.filter((p) => p.id !== providerId));
  };

  const handleAppriseEdit = (providerId: string, value: string) => {
    setAppriseEdits((prev) => ({ ...prev, [providerId]: value }));
    updateProviderState(providerId, (p) => ({ ...p, config: { ...p.config, urls: parseAppriseUrls(value) } }));
  };

  const handleAppriseNameEdit = (providerId: string, value: string) => {
    setAppriseNameEdits((prev) => ({ ...prev, [providerId]: value }));
    updateProviderState(providerId, (p) => ({ ...p, name: value }));
  };

  const openAppriseForm = (mode: AppriseFormMode, provider?: NotificationProvider) => {
    setAppriseFormMode(mode);
    setActiveAppriseId(provider?.id ?? null);
    if (provider) { setAppriseName(appriseNameEdits[provider.id] ?? provider.name); setAppriseUrls(appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls)); }
    else { setAppriseName(''); setAppriseUrls(''); }
    setShowAppriseForm(true);
  };

  const handleRequestPermission = async () => {
    if (typeof Notification === 'undefined') return;
    await Notification.requestPermission();
    bumpPermission();
  };

  const handleRefreshPermission = () => { if (typeof Notification !== 'undefined') bumpPermission(); };

  const handleTestNotification = async () => {
    if (typeof Notification === 'undefined') return;
    let permission = Notification.permission;
    if (permission === 'default') { permission = await Notification.requestPermission(); bumpPermission(); }
    if (permission !== 'granted') return;
    new Notification('Test notification', { body: 'If you can read this, browser notifications are working.' });
  };

  const closeAppriseForm = () => { setShowAppriseForm(false); setActiveAppriseId(null); };
  const handleTextareaInput = (event: FormEvent<HTMLTextAreaElement>) => { const t = event.currentTarget; t.style.height = 'auto'; t.style.height = `${t.scrollHeight}px`; };

  return { handleToggleEvent, handleCreateAppriseProvider, handleDeleteProvider, handleAppriseEdit, handleAppriseNameEdit, openAppriseForm, closeAppriseForm, handleTextareaInput, handleRequestPermission, handleRefreshPermission, handleTestNotification };
}

function useIsDirty(state: NotificationsState) {
  const { providers, baselineProviders, appriseEdits, appriseNameEdits, pendingDeletes } = state;
  return pendingDeletes.size > 0 || providers.some((provider) => {
    const baseline = baselineProviders.find((item) => item.id === provider.id);
    if (!baseline) return false;
    if (buildProviderUpdate(provider, baseline)) return true;
    if (provider.type === 'apprise') {
      const currentValue = appriseEdits[provider.id] ?? formatAppriseUrls(provider.config?.urls);
      const baselineValue = formatAppriseUrls(baseline.config?.urls);
      const nameValue = appriseNameEdits[provider.id] ?? provider.name;
      return currentValue !== baselineValue || nameValue !== baseline.name;
    }
    return false;
  });
}

type ExternalProvidersSectionProps = {
  appriseAvailable: boolean; appriseProviders: NotificationProvider[]; appriseFormMode: AppriseFormMode;
  activeAppriseId: string | null; appriseName: string; appriseUrls: string; showAppriseForm: boolean;
  setAppriseName: (v: string) => void; setAppriseUrls: (v: string) => void;
  onAppriseNameEdit: (id: string, v: string) => void; onAppriseEdit: (id: string, v: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void; onCloseForm: () => void;
  onDeleteProvider: (id: string) => void; onTextareaInput: (e: FormEvent<HTMLTextAreaElement>) => void;
  onCreateAppriseProvider: () => Promise<void>;
};

function ExternalProvidersSection({ appriseAvailable, appriseProviders, appriseFormMode, activeAppriseId, appriseName, appriseUrls, showAppriseForm, setAppriseName, setAppriseUrls, onAppriseNameEdit, onAppriseEdit, onOpenForm, onCloseForm, onDeleteProvider, onTextareaInput, onCreateAppriseProvider }: ExternalProvidersSectionProps) {
  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-medium">External Providers</div>
        <p className="text-sm text-muted-foreground">Configure external providers for remote notifications.</p>
      </div>
      {!appriseAvailable && (
        <p className="text-sm text-muted-foreground">
          Apprise is not installed yet. You can add it later to enable remote notifications.{' '}
          <a className="underline" href="https://github.com/caronc/apprise?tab=readme-ov-file#installation" target="_blank" rel="noreferrer">View installation instructions</a>.
        </p>
      )}
      {appriseProviders.length === 0 && <p className="text-sm text-muted-foreground">No Apprise providers configured yet.</p>}
      <AppriseProviderList
        providers={appriseProviders} appriseFormMode={appriseFormMode} activeAppriseId={activeAppriseId}
        appriseName={appriseName} appriseUrls={appriseUrls} onNameChange={setAppriseName} onUrlsChange={setAppriseUrls}
        onAppriseNameEdit={onAppriseNameEdit} onAppriseEdit={onAppriseEdit} onOpenForm={onOpenForm} onCloseForm={onCloseForm}
        onDeleteProvider={onDeleteProvider} onTextareaInput={onTextareaInput}
      />
      {appriseAvailable && (
        <div className="space-y-3">
          <Button variant="outline" className="cursor-pointer" onClick={() => onOpenForm('create')}>Add Apprise Provider</Button>
          {showAppriseForm && appriseFormMode === 'create' && (
            <AppriseProviderForm mode="create" name={appriseName} urls={appriseUrls} onNameChange={setAppriseName} onUrlsChange={setAppriseUrls}
              onSubmit={async () => { await onCreateAppriseProvider(); onCloseForm(); }} onCancel={onCloseForm} onInput={onTextareaInput} />
          )}
        </div>
      )}
    </div>
  );
}

export function NotificationsSettings() {
  const state = useNotificationsState();
  const { notificationPermission, bumpPermission } = useNotificationPermission();
  const saveRequest = useSaveRequest(state);
  const { handleToggleEvent, handleCreateAppriseProvider, handleDeleteProvider, handleAppriseEdit, handleAppriseNameEdit, openAppriseForm, closeAppriseForm, handleTextareaInput, handleRequestPermission, handleRefreshPermission, handleTestNotification } = useNotificationsActions(state, bumpPermission);
  const isDirty = useIsDirty(state);
  const { providers, appriseAvailable, appriseName, setAppriseName, appriseUrls, setAppriseUrls, showAppriseForm, appriseFormMode, activeAppriseId, notificationEvents } = state;

  const appriseProviders = providers.filter((provider) => provider.type === 'apprise');
  const tableProviders = useMemo(() => [...providers].sort((a, b) => {
    if (a.type === b.type) return a.name.localeCompare(b.name);
    if (a.type === 'local') return -1;
    if (b.type === 'local') return 1;
    return a.type.localeCompare(b.type);
  }), [providers]);
  const tableEvents = useMemo(() => {
    if (notificationEvents.length > 0) return notificationEvents;
    const eventSet = new Set<string>();
    for (const provider of providers) { for (const event of provider.events ?? []) { eventSet.add(event); } }
    return eventSet.size ? Array.from(eventSet) : DEFAULT_NOTIFICATION_EVENTS;
  }, [notificationEvents, providers]);

  const handleSave = async () => {
    try { await saveRequest.run(); } catch (error) { console.error('[NotificationsSettings] Failed to save notifications', error); }
  };

  return (
    <SettingsPageTemplate title="Notifications" description="Configure providers and choose which events should alert you." isDirty={isDirty} saveStatus={saveRequest.status} onSave={handleSave}>
      <DesktopNotificationsSection notificationPermission={notificationPermission} onRequestPermission={handleRequestPermission} onRefreshPermission={handleRefreshPermission} onTestNotification={handleTestNotification} />
      <Separator className='my-4' />
      <ExternalProvidersSection
        appriseAvailable={appriseAvailable} appriseProviders={appriseProviders} appriseFormMode={appriseFormMode}
        activeAppriseId={activeAppriseId} appriseName={appriseName} appriseUrls={appriseUrls} showAppriseForm={showAppriseForm}
        setAppriseName={setAppriseName} setAppriseUrls={setAppriseUrls}
        onAppriseNameEdit={handleAppriseNameEdit} onAppriseEdit={handleAppriseEdit}
        onOpenForm={openAppriseForm} onCloseForm={closeAppriseForm} onDeleteProvider={handleDeleteProvider}
        onTextareaInput={handleTextareaInput} onCreateAppriseProvider={handleCreateAppriseProvider}
      />
      <Separator className='my-4' />
      <div className="space-y-4">
        <div>
          <div className="text-base font-medium">Notification Events</div>
          <p className="text-sm text-muted-foreground">Select which providers should receive each notification type.</p>
        </div>
        {tableProviders.length === 0 && <p className="text-sm text-muted-foreground">No providers configured yet.</p>}
        {tableProviders.length > 0 && <NotificationEventsTable tableProviders={tableProviders} tableEvents={tableEvents} onToggleEvent={handleToggleEvent} />}
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
