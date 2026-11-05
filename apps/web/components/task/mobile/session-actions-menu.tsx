"use client";
import { useRef, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconDotsVertical } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
} from "@/hooks/domains/session/use-session-actions";
import type { TaskSessionState } from "@/lib/types/http";
import { HandoffDropdownMenuSub } from "../handoff-profile-menu-items";
import { useSessionMenuOpeningGesture } from "./use-session-menu-opening-gesture";

type SessionActionsMenuProps = {
  triggerRef: RefObject<HTMLButtonElement | null>;
  taskId: string;
  state: TaskSessionState | null;
  isPrimary: boolean;
  onSetPrimary: () => void;
  onStop: () => void;
  onResume: () => void;
  onRestart: () => void;
  isRestarting: boolean;
  onAskDelete: () => void;
  onExecution: () => void;
  onHandoffProfile: (profileId: string) => void;
};

/** Exposes touch-accessible session actions and blocks conflicting lifecycle requests during restart. */
export function SessionActionsMenu({
  triggerRef,
  taskId,
  state,
  isPrimary,
  onSetPrimary,
  onStop,
  onResume,
  onRestart,
  isRestarting,
  onAskDelete,
  onExecution,
  onHandoffProfile,
}: SessionActionsMenuProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const pending = useDeferredMenuActions(onExecution, onAskDelete);
  const { open, setOpen, triggerProps } = useSessionMenuOpeningGesture();
  const hasLifecycleAction =
    !!state &&
    (isSessionStoppable(state) || isSessionResumable(state) || isSessionDeletable(state));

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          ref={triggerRef}
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer h-11 w-11 md:h-7 md:w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          {...triggerProps}
          aria-label={t("task:sessionActions")}
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onClick={(e) => e.stopPropagation()}
        onCloseAutoFocus={pending.onCloseAutoFocus}
      >
        <DropdownMenuItem
          onSelect={() => {
            pending.execution.current = true;
          }}
        >
          {t("agents:executionDetails")}
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer"
          onSelect={onSetPrimary}
          disabled={isPrimary || !state}
        >
          {t("task:setAsPrimary")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {state && isSessionStoppable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onStop} disabled={isRestarting}>
            {t("task:stop")}
          </DropdownMenuItem>
        )}
        {(isRestarting || (state && isSessionStoppable(state))) && (
          <DropdownMenuItem
            className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onSelect={onRestart}
            disabled={isRestarting}
          >
            {t("task:restartSession")}
          </DropdownMenuItem>
        )}
        {state && isSessionResumable(state) && (
          <DropdownMenuItem className="cursor-pointer" onSelect={onResume} disabled={isRestarting}>
            {t("task:resume")}
          </DropdownMenuItem>
        )}
        {state && isSessionDeletable(state) && (
          <DropdownMenuItem
            className="cursor-pointer text-destructive focus:text-destructive"
            disabled={isRestarting}
            onSelect={() => {
              if (isMobile) pending.delete.current = true;
              else onAskDelete();
            }}
          >
            {t("task:delete")}
          </DropdownMenuItem>
        )}
        {hasLifecycleAction && <DropdownMenuSeparator />}
        <HandoffDropdownMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function useDeferredMenuActions(onExecution: () => void, onDelete: () => void) {
  const execution = useRef(false);
  const deletion = useRef(false);
  const onCloseAutoFocus = (event: Event) => {
    if (execution.current) {
      execution.current = false;
      event.preventDefault();
      queueMicrotask(onExecution);
    } else if (deletion.current) {
      deletion.current = false;
      event.preventDefault();
      onDelete();
    }
  };
  return { execution, delete: deletion, onCloseAutoFocus };
}
