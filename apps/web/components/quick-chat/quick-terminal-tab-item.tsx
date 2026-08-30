"use client";

import { memo } from "react";
import { IconAlertCircle, IconTerminal2, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  QuickChatTabActionMenu,
  QuickChatTabMoveButtons,
  type QuickChatTabDragProps,
} from "./quick-chat-tab-item";

type QuickTerminalTabItemProps = {
  sequence: number;
  isActive: boolean;
  error?: string;
  onActivate: () => void;
  onClose: () => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft?: boolean;
  canMoveRight?: boolean;
  dragProps?: QuickChatTabDragProps;
};

type QuickTerminalTabActionsProps = {
  label: string;
  closeLabel: string;
  onClose: () => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft: boolean;
  canMoveRight: boolean;
};

function QuickTerminalTabActions({
  label,
  closeLabel,
  onClose,
  onMoveLeft,
  onMoveRight,
  canMoveLeft,
  canMoveRight,
}: QuickTerminalTabActionsProps) {
  const { isFinePointer } = useResponsiveBreakpoint();

  return (
    <div className="flex shrink-0 items-center">
      {!isFinePointer && (
        <QuickChatTabActionMenu
          name={label}
          closeLabel={closeLabel}
          onMoveLeft={onMoveLeft}
          onMoveRight={onMoveRight}
          canMoveLeft={canMoveLeft}
          canMoveRight={canMoveRight}
          onClose={onClose}
        />
      )}
      {isFinePointer && onMoveLeft && onMoveRight && (
        <QuickChatTabMoveButtons
          name={label}
          onMoveLeft={onMoveLeft}
          onMoveRight={onMoveRight}
          canMoveLeft={canMoveLeft}
          canMoveRight={canMoveRight}
        />
      )}
      {isFinePointer && (
        <button
          type="button"
          aria-label={closeLabel}
          title={closeLabel}
          className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-60 hover:opacity-100 sm:h-6 sm:w-6"
          onClick={onClose}
        >
          <IconX className="h-3 w-3" aria-hidden />
        </button>
      )}
    </div>
  );
}

/** Fixed-label tab for a browser-local quick terminal. */
export const QuickTerminalTabItem = memo(function QuickTerminalTabItem({
  sequence,
  isActive,
  error,
  onActivate,
  onClose,
  onMoveLeft,
  onMoveRight,
  canMoveLeft = true,
  canMoveRight = true,
  dragProps,
}: QuickTerminalTabItemProps) {
  const { t } = useTranslation();
  const label = t("sidebar:quickChatTerminalTab", { count: sequence });
  const closeLabel = t("sidebar:quickChatCloseTerminal", { count: sequence });

  return (
    <div
      ref={dragProps?.setActivatorNodeRef}
      {...(dragProps?.attributes ?? {})}
      {...(dragProps?.listeners ?? {})}
      data-testid="quick-terminal-tab"
      data-terminal-sequence={sequence}
      className={`flex shrink-0 items-center gap-1 rounded transition-colors ${
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:bg-muted"
      } ${dragProps ? "cursor-grab active:cursor-grabbing" : ""}`}
    >
      <button
        type="button"
        onClick={onActivate}
        aria-label={label}
        aria-current={isActive ? "page" : undefined}
        className="flex h-11 min-w-0 cursor-pointer items-center gap-1.5 px-2.5 text-xs sm:h-6"
      >
        <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span>{label}</span>
        {error && <IconAlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />}
      </button>
      <QuickTerminalTabActions
        label={label}
        closeLabel={closeLabel}
        onClose={onClose}
        onMoveLeft={onMoveLeft}
        onMoveRight={onMoveRight}
        canMoveLeft={canMoveLeft}
        canMoveRight={canMoveRight}
      />
    </div>
  );
});
