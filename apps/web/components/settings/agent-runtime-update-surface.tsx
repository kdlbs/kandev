"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";

const UPDATE_AGENT_KEY = "agents:updateAgent";

export function AgentRuntimeUpdateSurface({
  agentName,
  displayName,
  isMobile,
  open,
  onOpenChange,
  body,
  footer,
  managedFallback,
  onBack,
  backLabel,
}: {
  agentName: string;
  displayName: string;
  isMobile: boolean;
  open: boolean;
  onOpenChange: (nextOpen: boolean) => void;
  body: ReactNode;
  footer: (mobile?: boolean) => ReactNode;
  managedFallback?: boolean;
  onBack?: () => void;
  backLabel?: string;
}) {
  const { t } = useTranslation();
  const description = t(
    managedFallback ? "agents:runtimeFallbackReview" : "agents:reviewUpdateBeforeApplying",
  );
  const back = onBack ? (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="cursor-pointer self-start px-2 max-md:min-h-11 max-md:text-sm [@media(pointer:coarse)]:min-h-11"
      onClick={onBack}
      data-testid={`agent-update-back-${agentName}`}
    >
      <IconChevronLeft className="mr-1 size-4" aria-hidden="true" />
      {backLabel ?? t("agents:runtimeUpdateBackToAll")}
    </Button>
  ) : null;
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="flex max-h-[92dvh] flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:max-h-[92dvh]"
          data-testid={`agent-update-drawer-${agentName}`}
        >
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            {back}
            <DrawerTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DrawerTitle>
            <DrawerDescription className="sr-only">{description}</DrawerDescription>
          </DrawerHeader>
          {body}
          {footer(true)}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:min-w-[40rem] sm:max-w-2xl"
        data-testid={`agent-update-dialog-${agentName}`}
      >
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3">
          {back}
          <DialogTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DialogTitle>
          <DialogDescription className="sr-only">{description}</DialogDescription>
        </DialogHeader>
        {body}
        {footer()}
      </DialogContent>
    </Dialog>
  );
}
