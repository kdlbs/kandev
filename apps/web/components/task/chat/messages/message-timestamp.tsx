"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useAppStore } from "@/components/state-provider";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { formatMessageTime } from "@/lib/i18n/message-time";
import { cn } from "@/lib/utils";

export function MessageTimestamp({
  createdAt,
  wrap = false,
}: {
  createdAt: string;
  wrap?: boolean;
}) {
  const { t } = useTranslation();
  const messageTimeDisplay = useAppStore((state) => state.userSettings.messageTimeDisplay);
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const formatted = formatMessageTime(createdAt, messageTimeDisplay);
  if (!formatted) return null;

  const wrapClassName = wrap ? "block max-w-full break-words tabular-nums" : undefined;
  const timeEl = (
    <time
      dateTime={createdAt}
      title={formatted.counterpart}
      aria-label={t("task:messageTimestampAriaLabel", {
        label: formatted.label,
        counterpart: formatted.counterpart,
      })}
      className={cn("min-w-0 text-[10px] text-muted-foreground/60 font-mono", wrapClassName)}
    >
      {formatted.label}
    </time>
  );
  if (!usesTouchDrawer) return timeEl;

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <button
          type="button"
          data-testid="message-timestamp-trigger"
          className={cn(
            "min-w-0 cursor-pointer border-0 bg-transparent p-0 text-left max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11",
            wrapClassName,
          )}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("task:messageTimestampAriaLabel", {
            label: formatted.label,
            counterpart: formatted.counterpart,
          })}
        >
          {timeEl}
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="message-timestamp-drawer" data-queue-escape-boundary>
        <DrawerHeader>
          <DrawerTitle>{t("task:messageTime")}</DrawerTitle>
          <DrawerDescription>{formatted.counterpart}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}
