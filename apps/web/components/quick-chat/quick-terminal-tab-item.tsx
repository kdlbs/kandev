"use client";

import { memo } from "react";
import {
  IconAlertCircle,
  IconChevronLeft,
  IconChevronRight,
  IconTerminal2,
  IconX,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { QuickChatTabActionMenu, type QuickChatTabDragHandleProps } from "./quick-chat-tab-item";

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
  dragHandleProps?: QuickChatTabDragHandleProps;
};

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
  dragHandleProps,
}: QuickTerminalTabItemProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const label = t("sidebar:quickChatTerminalTab", { count: sequence });
  const closeLabel = t("sidebar:quickChatCloseTerminal", { count: sequence });

  return (
    <div
      data-testid="quick-terminal-tab"
      data-terminal-sequence={sequence}
      className={`flex shrink-0 items-center gap-1 rounded transition-colors ${
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:bg-muted"
      }`}
    >
      <button
        type="button"
        ref={dragHandleProps?.setActivatorNodeRef}
        {...dragHandleProps?.attributes}
        {...dragHandleProps?.listeners}
        onClick={onActivate}
        aria-label={label}
        aria-current={isActive ? "page" : undefined}
        className="flex h-11 min-w-0 cursor-pointer items-center gap-1.5 px-2.5 text-xs sm:h-6"
      >
        <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span>{label}</span>
        {error && <IconAlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />}
      </button>
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
          <>
            <button
              type="button"
              aria-label={t("chat:moveQuickChatTabLeft", { name: label })}
              className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-70 hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-30 sm:hidden"
              disabled={!canMoveLeft}
              onClick={onMoveLeft}
            >
              <IconChevronLeft className="h-4 w-4" aria-hidden />
            </button>
            <button
              type="button"
              aria-label={t("chat:moveQuickChatTabRight", { name: label })}
              className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-70 hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-30 sm:hidden"
              disabled={!canMoveRight}
              onClick={onMoveRight}
            >
              <IconChevronRight className="h-4 w-4" aria-hidden />
            </button>
          </>
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
    </div>
  );
});
