"use client";

import { memo } from "react";
import { IconAlertCircle, IconTerminal2, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

type QuickTerminalTabItemProps = {
  sequence: number;
  isActive: boolean;
  error?: string;
  onActivate: () => void;
  onClose: () => void;
};

/** Fixed-label tab for a browser-local quick terminal. */
export const QuickTerminalTabItem = memo(function QuickTerminalTabItem({
  sequence,
  isActive,
  error,
  onActivate,
  onClose,
}: QuickTerminalTabItemProps) {
  const { t } = useTranslation();
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
        onClick={onActivate}
        aria-label={label}
        aria-current={isActive ? "page" : undefined}
        className="flex h-11 min-w-0 cursor-pointer items-center gap-1.5 px-2.5 text-xs sm:h-6"
      >
        <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span>{label}</span>
        {error && <IconAlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />}
      </button>
      <button
        type="button"
        aria-label={closeLabel}
        title={closeLabel}
        className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-60 hover:opacity-100 sm:h-6 sm:w-6"
        onClick={onClose}
      >
        <IconX className="h-3 w-3" aria-hidden />
      </button>
    </div>
  );
});
