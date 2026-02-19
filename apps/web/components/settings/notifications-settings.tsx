"use client";

import { useMemo, useReducer, useSyncExternalStore, type FormEvent } from "react";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { DEFAULT_NOTIFICATION_EVENTS, EVENT_LABELS } from "@/lib/notifications/events";
import type { NotificationProvider } from "@/lib/types/http";
import {
  useNotificationsState,
  useSaveRequest,
  useNotificationsActions,
  useIsDirty,
  type NotificationsState,
  type AppriseFormMode,
} from "@/components/settings/notifications-settings-actions";

type DesktopNotificationsSectionProps = {
  notificationPermission: NotificationPermission | "unsupported";
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
            title="Enable desktop notifications"
            variant="default"
            size="sm"
            onClick={onRequestPermission}
            disabled={
              notificationPermission === "granted" || notificationPermission === "unsupported"
            }
            className={
              notificationPermission === "granted"
                ? "bg-emerald-500 text-white hover:bg-emerald-500"
                : undefined
            }
          >
            {notificationPermission === "granted" ? "Enabled" : "Enable"}
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
                className="cursor-pointer"
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

      {notificationPermission === "denied" && (
        <p className="text-sm text-amber-600">
          Notifications are blocked in your browser. Enable them in site settings, then click
          Refresh.
        </p>
      )}
      {notificationPermission === "unsupported" && (
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
              description: "Notify when this event occurs.",
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
        const isEditing = appriseFormMode === "edit" && activeAppriseId === provider.id;
        return (
          <div key={provider.id} className="rounded-lg border border-muted p-4 space-y-3">
            {isEditing ? (
              <AppriseProviderForm
                mode="edit"
                name={appriseName}
                urls={appriseUrls}
                onNameChange={(value) => {
                  onNameChange(value);
                  onAppriseNameEdit(provider.id, value);
                }}
                onUrlsChange={(value) => {
                  onUrlsChange(value);
                  onAppriseEdit(provider.id, value);
                }}
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
                  <Button
                    variant="outline"
                    size="sm"
                    className="cursor-pointer"
                    onClick={() => onOpenForm("edit", provider)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="cursor-pointer"
                    onClick={() => onDeleteProvider(provider.id)}
                  >
                    Remove
                  </Button>
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
  const mounted = useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false,
  );
  const [, bumpPermission] = useReducer((value) => value + 1, 0);
  let notificationPermission: NotificationPermission | "unsupported";
  if (!mounted) notificationPermission = "default";
  else if (typeof Notification === "undefined") notificationPermission = "unsupported";
  else notificationPermission = Notification.permission;
  return { notificationPermission, bumpPermission };
}

type ExternalProvidersSectionProps = {
  appriseAvailable: boolean;
  appriseProviders: NotificationProvider[];
  appriseFormMode: AppriseFormMode;
  activeAppriseId: string | null;
  appriseName: string;
  appriseUrls: string;
  showAppriseForm: boolean;
  setAppriseName: (v: string) => void;
  setAppriseUrls: (v: string) => void;
  onAppriseNameEdit: (id: string, v: string) => void;
  onAppriseEdit: (id: string, v: string) => void;
  onOpenForm: (mode: AppriseFormMode, provider?: NotificationProvider) => void;
  onCloseForm: () => void;
  onDeleteProvider: (id: string) => void;
  onTextareaInput: (e: FormEvent<HTMLTextAreaElement>) => void;
  onCreateAppriseProvider: () => Promise<void>;
};

function ExternalProvidersSection({
  appriseAvailable,
  appriseProviders,
  appriseFormMode,
  activeAppriseId,
  appriseName,
  appriseUrls,
  showAppriseForm,
  setAppriseName,
  setAppriseUrls,
  onAppriseNameEdit,
  onAppriseEdit,
  onOpenForm,
  onCloseForm,
  onDeleteProvider,
  onTextareaInput,
  onCreateAppriseProvider,
}: ExternalProvidersSectionProps) {
  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-medium">External Providers</div>
        <p className="text-sm text-muted-foreground">
          Configure external providers for remote notifications.
        </p>
      </div>
      {!appriseAvailable && (
        <p className="text-sm text-muted-foreground">
          Apprise is not installed yet. You can add it later to enable remote notifications.{" "}
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
      <AppriseProviderList
        providers={appriseProviders}
        appriseFormMode={appriseFormMode}
        activeAppriseId={activeAppriseId}
        appriseName={appriseName}
        appriseUrls={appriseUrls}
        onNameChange={setAppriseName}
        onUrlsChange={setAppriseUrls}
        onAppriseNameEdit={onAppriseNameEdit}
        onAppriseEdit={onAppriseEdit}
        onOpenForm={onOpenForm}
        onCloseForm={onCloseForm}
        onDeleteProvider={onDeleteProvider}
        onTextareaInput={onTextareaInput}
      />
      {appriseAvailable && (
        <div className="space-y-3">
          <Button variant="outline" className="cursor-pointer" onClick={() => onOpenForm("create")}>
            Add Apprise Provider
          </Button>
          {showAppriseForm && appriseFormMode === "create" && (
            <AppriseProviderForm
              mode="create"
              name={appriseName}
              urls={appriseUrls}
              onNameChange={setAppriseName}
              onUrlsChange={setAppriseUrls}
              onSubmit={async () => {
                await onCreateAppriseProvider();
                onCloseForm();
              }}
              onCancel={onCloseForm}
              onInput={onTextareaInput}
            />
          )}
        </div>
      )}
    </div>
  );
}

function useTableData(state: NotificationsState) {
  const { providers, notificationEvents } = state;
  const tableProviders = useMemo(
    () =>
      [...providers].sort((a, b) => {
        if (a.type === b.type) return a.name.localeCompare(b.name);
        if (a.type === "local") return -1;
        if (b.type === "local") return 1;
        return a.type.localeCompare(b.type);
      }),
    [providers],
  );
  const tableEvents = useMemo(() => {
    if (notificationEvents.length > 0) return notificationEvents;
    const eventSet = new Set<string>();
    for (const provider of providers) {
      for (const event of provider.events ?? []) eventSet.add(event);
    }
    return eventSet.size ? Array.from(eventSet) : DEFAULT_NOTIFICATION_EVENTS;
  }, [notificationEvents, providers]);
  return { tableProviders, tableEvents };
}

export function NotificationsSettings() {
  const state = useNotificationsState();
  const { notificationPermission, bumpPermission } = useNotificationPermission();
  const saveRequest = useSaveRequest(state);
  const actions = useNotificationsActions(state, bumpPermission);
  const isDirty = useIsDirty(state);
  const { tableProviders, tableEvents } = useTableData(state);
  const {
    providers,
    appriseAvailable,
    appriseName,
    setAppriseName,
    appriseUrls,
    setAppriseUrls,
    showAppriseForm,
    appriseFormMode,
    activeAppriseId,
  } = state;
  const appriseProviders = providers.filter((provider) => provider.type === "apprise");

  const handleSave = async () => {
    try {
      await saveRequest.run();
    } catch (error) {
      console.error("[NotificationsSettings] Failed to save notifications", error);
    }
  };

  return (
    <SettingsPageTemplate
      title="Notifications"
      description="Configure providers and choose which events should alert you."
      isDirty={isDirty}
      saveStatus={saveRequest.status}
      onSave={handleSave}
    >
      <DesktopNotificationsSection
        notificationPermission={notificationPermission}
        onRequestPermission={actions.handleRequestPermission}
        onRefreshPermission={actions.handleRefreshPermission}
        onTestNotification={actions.handleTestNotification}
      />
      <Separator className="my-4" />
      <ExternalProvidersSection
        appriseAvailable={appriseAvailable}
        appriseProviders={appriseProviders}
        appriseFormMode={appriseFormMode}
        activeAppriseId={activeAppriseId}
        appriseName={appriseName}
        appriseUrls={appriseUrls}
        showAppriseForm={showAppriseForm}
        setAppriseName={setAppriseName}
        setAppriseUrls={setAppriseUrls}
        onAppriseNameEdit={actions.handleAppriseNameEdit}
        onAppriseEdit={actions.handleAppriseEdit}
        onOpenForm={actions.openAppriseForm}
        onCloseForm={actions.closeAppriseForm}
        onDeleteProvider={actions.handleDeleteProvider}
        onTextareaInput={handleTextareaInput}
        onCreateAppriseProvider={actions.handleCreateAppriseProvider}
      />
      <Separator className="my-4" />
      <div className="space-y-4">
        <div>
          <div className="text-base font-medium">Notification Events</div>
          <p className="text-sm text-muted-foreground">
            Select which providers should receive each notification type.
          </p>
        </div>
        {tableProviders.length > 0 && (
          <NotificationEventsTable
            tableProviders={tableProviders}
            tableEvents={tableEvents}
            onToggleEvent={actions.handleToggleEvent}
          />
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
          {mode === "create" ? "Add provider" : "Done"}
        </Button>
        <Button variant="ghost" className="cursor-pointer" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

function handleTextareaInput(event: FormEvent<HTMLTextAreaElement>) {
  const t = event.currentTarget;
  t.style.height = "auto";
  t.style.height = `${t.scrollHeight}px`;
}
