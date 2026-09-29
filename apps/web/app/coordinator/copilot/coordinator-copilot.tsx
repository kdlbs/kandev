"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconMessageChatbot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ChatPopoverShell } from "@/components/config-chat/chat-popover-shell";
import { useCoordinatorCopilot } from "./use-coordinator-copilot";
import { CoordinatorCopilotBody } from "./coordinator-copilot-body";

export type CoordinatorCopilotProps = {
  workspaceId: string;
  coordinatorId: string;
  coordinatorName: string;
  canManage: boolean;
};

/**
 * The coordinator copilot launcher and popover, mounted on both Coordinator
 * screens for the viewed coordinator
 * (docs/specs/coordinator/system-design/copilot-popover.md#popover). Renders
 * nothing, and issues no request, unless `features.coordinator` is on and
 * the viewer holds `workspace.manage`.
 */
export function CoordinatorCopilot({
  workspaceId,
  coordinatorId,
  coordinatorName,
  canManage,
}: CoordinatorCopilotProps) {
  const { t } = useTranslation();
  const copilot = useCoordinatorCopilot(workspaceId, coordinatorId, canManage);
  // Set right before closing the popover to navigate a chat card's Edit/Reject
  // to its Needs-you deep link (proposal-cards.md#cards "Forms and
  // navigation"): Radix's default `onCloseAutoFocus` would otherwise return
  // focus to the launcher button once the popover unmounts, racing with (and
  // sometimes winning against) the deep-linked form's own auto-focus. Escape
  // and the header Close button call `handleOpenChange` directly and never
  // set this, so they keep the default return-to-launcher focus (AC .004.7).
  const suppressCloseAutoFocusRef = useRef(false);

  if (!copilot.enabled) return null;

  const launcherLabel = copilot.launcher.busy
    ? t("coordinator:copilotLauncherBusy", { name: coordinatorName })
    : t("coordinator:copilotLauncherIdle", { name: coordinatorName });

  return (
    <ChatPopoverShell
      open={copilot.open}
      onOpenChange={copilot.handleOpenChange}
      testId="coordinator-copilot-popover"
      icon={<IconMessageChatbot className="h-4 w-4 shrink-0 text-muted-foreground" />}
      title={t("coordinator:copilotTitle", { name: coordinatorName })}
      closeLabel={t("coordinator:copilotClose")}
      onCloseAutoFocus={(event) => {
        if (!suppressCloseAutoFocusRef.current) return;
        suppressCloseAutoFocusRef.current = false;
        event.preventDefault();
      }}
      trigger={
        <Tooltip open={copilot.open ? false : undefined}>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button
                size="icon"
                className="fixed bottom-[calc(1.5rem+var(--app-status-bar-height))] right-6 z-50 size-12 max-md:size-12 [@media(pointer:coarse)]:size-12 cursor-pointer rounded-full shadow-lg"
                aria-label={launcherLabel}
                data-testid="coordinator-copilot-launcher"
                data-busy={copilot.launcher.busy}
              >
                <IconMessageChatbot className="h-6 w-6" />
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent side="left">{launcherLabel}</TooltipContent>
        </Tooltip>
      }
    >
      <CoordinatorCopilotBody
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        state={copilot.openSequence.state}
        routeSession={copilot.routeSession}
        chip={copilot.chip}
        pendingDraft={copilot.pendingDraft}
        askKey={copilot.askKey}
        onRetry={copilot.openSequence.retry}
        onRemoveChip={copilot.removeChip}
        onSuggest={copilot.suggest}
        onClosePopover={() => {
          suppressCloseAutoFocusRef.current = true;
          copilot.handleOpenChange(false);
        }}
      />
    </ChatPopoverShell>
  );
}
