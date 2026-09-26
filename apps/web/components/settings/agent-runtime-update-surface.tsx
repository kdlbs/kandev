"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
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
}: {
  agentName: string;
  displayName: string;
  isMobile: boolean;
  open: boolean;
  onOpenChange: (nextOpen: boolean) => void;
  body: ReactNode;
  footer: (mobile?: boolean) => ReactNode;
}) {
  const { t } = useTranslation();
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="max-h-[92dvh] overflow-hidden data-[vaul-drawer-direction=bottom]:max-h-[92dvh]"
          data-testid={`agent-update-drawer-${agentName}`}
        >
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DrawerTitle>
            <DrawerDescription>{t("agents:reviewUpdateBeforeApplying")}</DrawerDescription>
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
        className="max-h-[92dvh] gap-0 overflow-hidden p-0 sm:min-w-[40rem] sm:max-w-2xl"
        data-testid={`agent-update-dialog-${agentName}`}
      >
        <DialogHeader className="px-4 pb-1 pt-3">
          <DialogTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DialogTitle>
          <DialogDescription>{t("agents:reviewUpdateBeforeApplying")}</DialogDescription>
        </DialogHeader>
        {body}
        {footer()}
      </DialogContent>
    </Dialog>
  );
}
