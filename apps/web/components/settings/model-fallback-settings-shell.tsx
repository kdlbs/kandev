"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { cn } from "@/lib/utils";

export type FallbackOptionKind = "automatic" | "explicit";

function helpCopy(kind: FallbackOptionKind, t: (key: string) => string) {
  if (kind === "automatic") {
    return {
      label: t("settings:autoFallbackInfoLabel"),
      title: t("settings:autoFallbackHelpTitle"),
      body: t("settings:autoFallbackHelp"),
    };
  }
  return {
    label: t("settings:agentFallbackInfoLabel"),
    title: t("settings:agentFallbackHelpTitle"),
    body: t("settings:agentFallbackHelp"),
  };
}

export function FallbackOptionHelp({ kind }: { kind: FallbackOptionKind }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const copy = helpCopy(kind, t);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={copy.label}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
      data-testid={`profile-${kind}-fallback-help`}
      className={cn(
        "shrink-0 cursor-pointer text-muted-foreground hover:text-foreground",
        usesTouchDrawer ? "h-11 w-11" : "h-7 w-7",
      )}
    >
      <IconInfoCircle className="size-4" aria-hidden="true" />
    </Button>
  );
  const trigger = usesTouchDrawer ? (
    <DrawerTrigger asChild>{button}</DrawerTrigger>
  ) : (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent
          side="bottom"
          align="start"
          sideOffset={8}
          className="max-w-[min(24rem,calc(100vw-2rem))]"
        >
          <div className="space-y-1">
            <p className="font-medium">{copy.title}</p>
            <p>{copy.body}</p>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{copy.title}</DrawerTitle>
          <DrawerDescription>{copy.body}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

export function ModelFallbackSettingsShell({
  autoFallback,
  fallbackModel,
  isDirty,
  automaticOption,
  explicitOption,
}: {
  autoFallback: boolean;
  fallbackModel: string;
  isDirty?: boolean;
  automaticOption: ReactNode;
  explicitOption: ReactNode;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  let summary = t("settings:fallbackSettingsSummaryStrict");
  if (fallbackModel) {
    summary = t("settings:fallbackSettingsSummaryExplicit", { model: fallbackModel });
  }
  if (autoFallback) {
    summary = t("settings:fallbackSettingsSummaryAutomatic");
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="space-y-2"
      data-testid="profile-fallback-settings"
    >
      <div
        className="flex min-h-11 min-w-0 items-center gap-2"
        data-settings-dirty={isDirty}
        data-settings-dirty-level="container"
      >
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md px-1 text-left hover:bg-muted/40"
            data-testid="profile-fallback-settings-trigger"
          >
            <IconChevronRight
              className={cn(
                "size-4 shrink-0 text-muted-foreground transition-transform",
                open && "rotate-90",
              )}
              aria-hidden="true"
            />
            <span className="truncate font-medium">{t("settings:fallbackSettings")}</span>
          </button>
        </CollapsibleTrigger>
        <span
          className="min-w-0 truncate text-xs text-muted-foreground"
          data-testid="profile-fallback-settings-summary"
        >
          {summary}
        </span>
      </div>
      <CollapsibleContent>
        <div
          className="grid min-w-0 grid-cols-1 gap-3 md:grid-cols-2"
          data-testid="profile-fallback-settings-grid"
        >
          <div
            className="min-w-0 rounded-md border border-border/70 bg-muted/10 p-3"
            data-testid="profile-auto-fallback-option"
          >
            {automaticOption}
          </div>
          <div
            className="min-w-0 rounded-md border border-border/70 bg-muted/10 p-3"
            data-testid="profile-explicit-fallback-option"
          >
            {explicitOption}
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
