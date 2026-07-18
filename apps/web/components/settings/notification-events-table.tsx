"use client";

import { IconBell } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { EVENT_LABELS } from "@/lib/notifications/events";
import type { NotificationProvider } from "@/lib/types/http";

type Props = {
  tableProviders: NotificationProvider[];
  baselineProviders: NotificationProvider[];
  tableEvents: string[];
  onToggleEvent: (provider: NotificationProvider, eventType: string) => void;
  onTestProvider: (providerId: string) => Promise<void>;
};

export function NotificationEventsTable({
  tableProviders,
  baselineProviders,
  tableEvents,
  onToggleEvent,
  onTestProvider,
}: Props) {
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
                <div className="flex items-center justify-center gap-1.5">
                  <span>{provider.name}</span>
                  {provider.type !== "local" && (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-6 w-6 cursor-pointer"
                            aria-label={`Send test notification for ${provider.name}`}
                            onClick={() => void onTestProvider(provider.id)}
                          >
                            <IconBell className="h-3.5 w-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Send test notification</TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                </div>
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
                {tableProviders.map((provider) => {
                  const checked = (provider.events ?? []).includes(eventType);
                  const baselineChecked = (
                    baselineProviders.find((candidate) => candidate.id === provider.id)?.events ??
                    []
                  ).includes(eventType);
                  return (
                    <td key={provider.id} className="px-4 py-3 text-center">
                      <div className="flex justify-center">
                        <Checkbox
                          checked={checked}
                          data-settings-dirty={checked !== baselineChecked}
                          onCheckedChange={() => onToggleEvent(provider, eventType)}
                        />
                      </div>
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
