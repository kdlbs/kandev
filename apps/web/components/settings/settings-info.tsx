"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

export function SettingsInfo({
  label,
  children,
  testId,
}: {
  label: string;
  children: ReactNode;
  testId?: string;
}) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      data-testid={testId}
      data-settings-info="true"
      aria-label={t("settings:settingInfoLabel", { setting: label })}
      aria-haspopup={touch ? "dialog" : undefined}
      aria-expanded={touch ? open : undefined}
      className="size-6 shrink-0 cursor-pointer text-muted-foreground max-md:size-11 [@media(pointer:coarse)]:size-11"
    >
      <IconInfoCircle className="size-4" aria-hidden="true" />
    </Button>
  );
  if (touch)
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{button}</DrawerTrigger>
        <DrawerContent aria-describedby={undefined} className="max-h-[85dvh]">
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 text-sm leading-relaxed">
            <div className="space-y-3">{children}</div>
          </div>
          <div className="p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            <DrawerClose asChild>
              <Button variant="outline" className="h-11 w-full cursor-pointer">
                {t("common:close")}
              </Button>
            </DrawerClose>
          </div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent
          side="bottom"
          align="start"
          className="max-h-[70vh] max-w-[min(28rem,calc(100vw-2rem))] overflow-y-auto text-xs leading-relaxed"
        >
          <div className="space-y-2">{children}</div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
