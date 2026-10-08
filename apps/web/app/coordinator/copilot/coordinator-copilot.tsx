"use client";

import { useCallback, useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconMessageChatbot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { RightSidePanel } from "@/components/right-side-panel";
import { useCoordinatorCopilot } from "./use-coordinator-copilot";
import { useCopilotPanelWidth } from "./use-copilot-panel-width";
import { CoordinatorCopilotPanelContent } from "./coordinator-copilot-panel-content";

export type CoordinatorCopilotProps = {
  workspaceId: string;
  coordinatorId: string;
  coordinatorName: string;
  canManage: boolean;
  /** The screen content the panel sits beside. */
  children: ReactNode;
};

/**
 * The coordinator copilot launcher and right-side panel, wrapped around the
 * viewed coordinator's screen content
 * (docs/specs/coordinator/system-design/copilot-popover.md#popover). The
 * panel and launcher render only when `features.coordinator` is on and the
 * viewer holds `workspace.manage`; the content always renders.
 */
export function CoordinatorCopilot({
  workspaceId,
  coordinatorId,
  coordinatorName,
  canManage,
  children,
}: CoordinatorCopilotProps) {
  const { t } = useTranslation();
  const copilot = useCoordinatorCopilot(workspaceId, coordinatorId, canManage);
  const { widthPx, updateWidth } = useCopilotPanelWidth();
  const launcherRef = useRef<HTMLButtonElement>(null);
  // Set by Close, Escape and the backdrop so the launcher, which is hidden while
  // the panel is open, takes focus back once it remounts. A chat card's
  // Edit/Reject closes through `onClosePopover` without setting it, leaving the
  // deep-linked form's own auto-focus uncontested.
  const returnFocusRef = useRef(false);

  const { handleOpenChange } = copilot;
  const closeAndReturnFocus = useCallback(() => {
    returnFocusRef.current = true;
    handleOpenChange(false);
  }, [handleOpenChange]);

  const showPanel = copilot.enabled && copilot.open;
  useEffect(() => {
    if (showPanel || !returnFocusRef.current) return;
    returnFocusRef.current = false;
    launcherRef.current?.focus();
  }, [showPanel]);

  const launcherLabel = copilot.launcher.busy
    ? t("coordinator:copilotLauncherBusy", { name: coordinatorName })
    : t("coordinator:copilotLauncherIdle", { name: coordinatorName });

  return (
    <>
      <RightSidePanel
        open={showPanel}
        onClose={closeAndReturnFocus}
        widthPx={widthPx}
        onWidthChange={updateWidth}
        backdropLabel={t("coordinator:copilotClose")}
        mainSizing="fluid"
        mobileFullScreen
        closeOnEscape
        panelTestId="coordinator-copilot-popover"
        main={children}
      >
        <CoordinatorCopilotPanelContent
          copilot={copilot}
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          coordinatorName={coordinatorName}
          onClose={closeAndReturnFocus}
          onClosePopover={() => copilot.handleOpenChange(false)}
        />
      </RightSidePanel>
      {copilot.enabled && !copilot.open && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              ref={launcherRef}
              size="icon"
              className="fixed bottom-[calc(1.5rem+var(--app-status-bar-height))] right-6 z-50 size-12 max-md:size-12 [@media(pointer:coarse)]:size-12 cursor-pointer rounded-full shadow-lg"
              aria-label={launcherLabel}
              data-testid="coordinator-copilot-launcher"
              data-busy={copilot.launcher.busy}
              onClick={() => copilot.handleOpenChange(true)}
            >
              <IconMessageChatbot className="h-6 w-6" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">{launcherLabel}</TooltipContent>
        </Tooltip>
      )}
    </>
  );
}
