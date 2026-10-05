"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAdjustments, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { HideDisabledAgentProfilesSetting } from "@/app/settings/agents/hide-disabled-agent-profiles-setting";

function AgentOptionsBody({ isTouchTarget }: { isTouchTarget: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <HideDisabledAgentProfilesSetting isTouchTarget={isTouchTarget} />
      <p id="agent-options-description" className="text-xs text-muted-foreground">
        {t("agents:changesApplyImmediately")}
      </p>
    </div>
  );
}

export function AgentOptionsDialog() {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const isTouchTarget = isMobile || !isFinePointer;
  const triggerClassName = settingsActionClassName(
    isTouchTarget ? "min-h-11 cursor-pointer" : "cursor-pointer",
  );
  const trigger = (
    <Button variant="outline" className={triggerClassName} data-testid="agent-options-trigger">
      <IconAdjustments className="mr-2 h-4 w-4" />
      {t("agents:options")}
    </Button>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          data-testid="agent-options-drawer"
          aria-describedby="agent-options-description"
          className="!max-h-[min(80dvh,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))] overflow-hidden outline-none"
        >
          <div
            data-testid="agent-options-mobile-card"
            className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background shadow-2xl shadow-black/20"
          >
            <DrawerHeader className="shrink-0 border-b border-border/70 pb-3 text-left">
              <DrawerTitle>{t("agents:agentOptions")}</DrawerTitle>
            </DrawerHeader>
            <div
              data-testid="agent-options-mobile-scroll"
              className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]"
            >
              <AgentOptionsBody isTouchTarget={isTouchTarget} />
            </div>
            <DrawerFooter
              data-testid="agent-options-mobile-footer"
              className="shrink-0 border-t border-border/70 bg-background/95 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]"
            >
              <DrawerClose asChild>
                <Button className="h-12 min-h-12 w-full">{t("agents:done")}</Button>
              </DrawerClose>
            </DrawerFooter>
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent
        showCloseButton={false}
        data-testid="agent-options-dialog"
        aria-describedby="agent-options-description"
        className="sm:max-w-sm"
      >
        <DialogHeader className={isTouchTarget ? "pr-12" : "pr-8"}>
          <DialogTitle>{t("agents:agentOptions")}</DialogTitle>
        </DialogHeader>
        <DialogClose asChild>
          <Button
            variant="ghost"
            size={isTouchTarget ? "icon" : "icon-sm"}
            aria-label={t("common:close")}
            className="absolute top-2 right-2"
          >
            <IconX />
          </Button>
        </DialogClose>
        <AgentOptionsBody isTouchTarget={isTouchTarget} />
        <DialogFooter>
          <DialogClose asChild>
            <Button>{t("agents:done")}</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
