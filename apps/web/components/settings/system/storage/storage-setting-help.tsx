"use client";

import { useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

export function StorageSettingHelp({
  label,
  children,
  testId,
}: {
  label: string;
  children: string;
  testId?: string;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) {
      window.requestAnimationFrame(() => triggerRef.current?.focus());
    }
  };
  const button = (
    <Button
      ref={triggerRef}
      type="button"
      variant="ghost"
      size="icon-sm"
      className="size-11 shrink-0 cursor-help text-muted-foreground sm:size-7"
      data-testid={testId}
      aria-label={t("system:storageMoreInformationAbout", { label })}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
      onClick={() => {
        if (!usesTouchDrawer) setOpen((current) => !current);
      }}
    >
      <IconInfoCircle className="size-4" aria-hidden="true" />
    </Button>
  );
  const trigger = usesTouchDrawer ? (
    <DrawerTrigger asChild>{button}</DrawerTrigger>
  ) : (
    <TooltipProvider>
      <Tooltip open={!usesTouchDrawer ? open : undefined} onOpenChange={setOpen}>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent className="max-w-xs text-xs leading-relaxed">{children}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
  return (
    <Drawer open={usesTouchDrawer && open} onOpenChange={handleOpenChange}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{label}</DrawerTitle>
          <DrawerDescription>
            {t("system:storageMoreInformationAbout", { label })}
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-2 text-sm leading-relaxed">
          {children}
        </div>
        <DrawerFooter className="pb-[max(1rem,env(safe-area-inset-bottom))]">
          <DrawerClose asChild>
            <Button type="button" variant="outline" className="h-11 w-full">
              {t("system:close")}
            </Button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
