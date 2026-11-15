"use client";

import { useTranslation } from "react-i18next";
import { IconChevronLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import type { AgentUpdateStatus } from "@/lib/api";
import { RuntimePolicyDetail } from "./agent-runtime-policies";

/**
 * Read-only detail for an externally managed, unsupported, or vanished runtime.
 * Keeps one overlay for overview-to-detail navigation with an explicit Back.
 */
export function RuntimeUpdateGuidanceSurface({
  agentName,
  displayName,
  status,
  isMobile,
  onBack,
  onClose,
}: {
  agentName: string;
  displayName: string;
  status?: AgentUpdateStatus;
  isMobile: boolean;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const back = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="cursor-pointer self-start px-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      onClick={onBack}
      data-testid="runtime-guidance-back"
    >
      <IconChevronLeft className="mr-1 size-4" aria-hidden="true" />
      {t("agents:runtimeUpdateBackToAll")}
    </Button>
  );
  const body = (
    <>
      {status ? (
        <RuntimePolicyDetail status={status} />
      ) : (
        <div className="space-y-2 px-4 py-3">
          <p className="text-sm text-muted-foreground">{t("agents:runtimeFocusUnavailable")}</p>
        </div>
      )}
    </>
  );
  const footer = (
    <Button type="button" variant="outline" onClick={onClose}>
      {t("common:close")}
    </Button>
  );
  if (isMobile) {
    return (
      <Drawer open onOpenChange={(open) => !open && onClose()}>
        <DrawerContent
          className="flex max-h-[92dvh] flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:max-h-[92dvh]"
          data-testid={`runtime-guidance-drawer-${agentName}`}
        >
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            {back}
            <DrawerTitle>{displayName}</DrawerTitle>
            <DrawerDescription className="sr-only">
              {t("agents:runtimeManagerDescription")}
            </DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto">{body}</div>
          <DrawerFooter className="border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {footer}
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:min-w-[34rem] sm:max-w-xl"
        data-testid={`runtime-guidance-dialog-${agentName}`}
      >
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3">
          {back}
          <DialogTitle>{displayName}</DialogTitle>
          <DialogDescription className="sr-only">
            {t("agents:runtimeManagerDescription")}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto">{body}</div>
        <DialogFooter className="border-t px-4 py-2">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
