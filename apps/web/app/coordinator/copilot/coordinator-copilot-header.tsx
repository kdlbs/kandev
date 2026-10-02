"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconMessageChatbot, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

export type CoordinatorCopilotHeaderProps = {
  coordinatorName: string;
  busy: boolean;
  onClose: () => void;
  /** Controls placed before Close (the workspace host's switcher and page link). */
  actions?: ReactNode;
};

/** The panel header: title, a busy status while the coordinator works, and Close. */
export function CoordinatorCopilotHeader({
  coordinatorName,
  busy,
  onClose,
  actions,
}: CoordinatorCopilotHeaderProps) {
  const { t } = useTranslation();
  return (
    <header className="flex h-12 shrink-0 items-center justify-between border-b bg-muted/30 pl-3">
      <div className="flex min-w-0 items-center gap-2">
        <IconMessageChatbot className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate text-sm font-medium">
          {t("coordinator:copilotTitle", { name: coordinatorName })}
        </span>
        {busy && (
          <span
            role="status"
            data-testid="coordinator-copilot-busy"
            className="text-muted-foreground shrink-0 text-xs"
          >
            {t("coordinator:copilotLauncherBusy", { name: coordinatorName })}
          </span>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {actions}
        <Button
          size="icon"
          variant="ghost"
          className="h-11 w-11 cursor-pointer rounded-none"
          onClick={onClose}
          aria-label={t("coordinator:copilotClose")}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </header>
  );
}
