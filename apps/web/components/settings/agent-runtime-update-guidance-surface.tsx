"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerFooter } from "@kandev/ui/drawer";
import type { AgentUpdateStatus } from "@/lib/api";
import { RuntimePolicyDetail } from "./agent-runtime-policies";
import { RuntimeUpdateDetailHeader } from "./agent-runtime-update-surface";

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
  const header = (
    <RuntimeUpdateDetailHeader
      isMobile={isMobile}
      title={displayName}
      description={t("agents:runtimeManagerDescription")}
      back={{ onClick: onBack, testId: "runtime-guidance-back" }}
    />
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
          {header}
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
        {header}
        <div className="min-h-0 flex-1 overflow-y-auto">{body}</div>
        <DialogFooter className="border-t px-4 py-2">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
