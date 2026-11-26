"use client";

import { IconHistory } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useTranslation } from "react-i18next";

export function SessionHistoryToggle({
  taskId,
  showHistory,
  mobile = false,
}: {
  taskId: string;
  showHistory: boolean;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  const setSessionHistoryVisible = useDockviewStore((state) => state.setSessionHistoryVisible);
  const label = showHistory ? t("task:hideSessionHistory") : t("task:showSessionHistory");
  const variant = mobile || !showHistory ? "ghost" : "secondary";
  return (
    <Button
      variant={variant}
      size={mobile ? "sm" : "icon"}
      className={
        mobile
          ? "min-h-11 w-full justify-start gap-2"
          : "h-7 w-7 shrink-0 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
      }
      onClick={() => setSessionHistoryVisible(taskId, !showHistory)}
      aria-label={label}
      title={label}
      data-testid={mobile ? "mobile-session-history-toggle" : "preview-session-history-toggle"}
    >
      <IconHistory className="h-4 w-4" />
      {mobile && label}
    </Button>
  );
}
