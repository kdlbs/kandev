"use client";

import { IconBell } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  NOTIFICATION_EVENT_OFFICE_INBOX_ITEM,
  NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
  NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
  NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE,
} from "@/lib/notifications/events";
import type { NotificationProvider } from "@/lib/types/http";

type Props = {
  tableProviders: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  tableEvents: string[];
  onToggleEvent: (provider: NotificationProvider, eventType: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
};

/**
 * Catalog keys for each known event type — keys, not resolved copy. The event
 * type itself is a wire sentinel and is never translated; only the row's title
 * and description are. Resolving these at module scope would pin the copy to the
 * boot locale (docs/i18n.md), so `useEventMeta` resolves them at render.
 *
 * An event type the backend introduces before this map knows it falls back to
 * the raw type plus a generic description, exactly as it did before i18n.
 */
const EVENT_MESSAGE_KEYS: Record<string, { title: string; description: string }> = {
  [NOTIFICATION_EVENT_SESSION_TURN_FINISHED]: {
    title: "settings:notificationEventTurnFinished",
    description: "settings:notificationEventTurnFinishedDescription",
  },
  [NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]: {
    title: "settings:notificationEventClarificationRequested",
    description: "settings:notificationEventClarificationRequestedDescription",
  },
  [NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]: {
    title: "settings:notificationEventUpdateAvailable",
    description: "settings:notificationEventUpdateAvailableDescription",
  },
  [NOTIFICATION_EVENT_OFFICE_INBOX_ITEM]: {
    title: "settings:notificationEventOfficeInboxItem",
    description: "settings:notificationEventOfficeInboxItemDescription",
  },
};

function useEventMeta() {
  const { t } = useTranslation();
  return (eventType: string) => {
    const keys = EVENT_MESSAGE_KEYS[eventType];
    return {
      title: keys ? t(keys.title) : eventType,
      description: keys ? t(keys.description) : t("settings:notifyWhenThisEventOccurs"),
    };
  };
}

/**
 * `eventTitle` comes from the row that owns it rather than a second
 * `useEventMeta()` lookup, so the accessible name can never drift from the title
 * the user reads next to the checkbox.
 */
function EventCheckbox({
  provider,
  baselineProviders,
  eventType,
  eventTitle,
  onToggleEvent,
  mobile = false,
}: {
  provider: NotificationProvider;
  baselineProviders: NotificationProvider[];
  eventType: string;
  eventTitle: string;
  onToggleEvent: Props["onToggleEvent"];
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  const checked = (provider.events ?? []).includes(eventType);
  const baselineChecked = (
    baselineProviders.find((candidate) => candidate.id === provider.id)?.events ?? []
  ).includes(eventType);
  const checkbox = (
    <Checkbox
      aria-label={t("settings:notificationEventToggle", {
        event: eventTitle,
        name: provider.name,
      })}
      checked={checked}
      data-settings-dirty={checked !== baselineChecked}
      onCheckedChange={() => onToggleEvent(provider, eventType)}
    />
  );
  if (!mobile) return checkbox;
  return (
    <label
      className="flex size-11 shrink-0 cursor-pointer items-center justify-center"
      data-testid={`notification-event-toggle-${eventType}-${provider.id}`}
    >
      {checkbox}
    </label>
  );
}

function TestProviderButton({
  provider,
  onTestProvider,
  mobile = false,
}: Pick<Props, "onTestProvider"> & {
  provider: NotificationProvider;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  if (provider.type === "local") return null;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={mobile ? "h-11 w-11 shrink-0 cursor-pointer" : "h-6 w-6 cursor-pointer"}
            aria-label={t("settings:sendTestNotificationFor", { name: provider.name })}
            onClick={() => void onTestProvider(provider.id)}
          >
            <IconBell className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("settings:sendTestNotification")}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function MobileEventList({
  tableProviders,
  baselineProviders,
  tableEvents,
  onToggleEvent,
  onTestProvider,
}: Props) {
  const eventMeta = useEventMeta();
  return (
    <div className="space-y-4 md:hidden" data-testid="notification-events-mobile-list">
      {tableEvents.map((eventType) => {
        const meta = eventMeta(eventType);
        return (
          <section key={eventType} className="space-y-3 rounded-lg border border-muted p-3">
            <div>
              <h3 className="font-medium">{meta.title}</h3>
              <p className="text-xs text-muted-foreground">{meta.description}</p>
            </div>
            <div className="space-y-2">
              {tableProviders.map((provider) => (
                <div
                  key={provider.id}
                  className="flex min-h-11 items-center gap-3 rounded-md border border-muted px-3 py-2"
                >
                  <span className="min-w-0 flex-1 break-words text-sm font-medium">
                    {provider.name}
                  </span>
                  <TestProviderButton provider={provider} onTestProvider={onTestProvider} mobile />
                  <EventCheckbox
                    provider={provider}
                    baselineProviders={baselineProviders}
                    eventType={eventType}
                    eventTitle={meta.title}
                    onToggleEvent={onToggleEvent}
                    mobile
                  />
                </div>
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function DesktopEventTable({
  tableProviders,
  baselineProviders,
  tableEvents,
  onToggleEvent,
  onTestProvider,
}: Props) {
  const { t } = useTranslation();
  const eventMeta = useEventMeta();
  return (
    <div
      className="hidden overflow-auto rounded-lg border border-muted md:block"
      data-testid="notification-events-desktop-table"
    >
      <table className="min-w-full text-sm">
        <thead className="bg-muted/40">
          <tr>
            <th className="px-4 py-3 text-left font-medium">{t("settings:notificationType")}</th>
            {tableProviders.map((provider) => (
              <th key={provider.id} className="px-4 py-3 text-center font-medium">
                <div className="flex items-center justify-center gap-1.5">
                  <span>{provider.name}</span>
                  <TestProviderButton provider={provider} onTestProvider={onTestProvider} />
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {tableEvents.map((eventType) => {
            const meta = eventMeta(eventType);
            return (
              <tr key={eventType} className="border-t border-muted">
                <td className="px-4 py-3">
                  <div className="font-medium">{meta.title}</div>
                  <div className="text-xs text-muted-foreground">{meta.description}</div>
                </td>
                {tableProviders.map((provider) => (
                  <td key={provider.id} className="px-4 py-3 text-center">
                    <div className="flex justify-center">
                      <EventCheckbox
                        provider={provider}
                        baselineProviders={baselineProviders}
                        eventType={eventType}
                        eventTitle={meta.title}
                        onToggleEvent={onToggleEvent}
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

export function NotificationEventsTable(props: Props) {
  const { t } = useTranslation();
  if (props.tableProviders.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t("settings:noProvidersConfiguredYet")}</p>
    );
  }

  return (
    <>
      <MobileEventList {...props} />
      <DesktopEventTable {...props} />
    </>
  );
}
