"use client";

import type { MouseEventHandler } from "react";
import { Button } from "@kandev/ui/button";
import { IconMenu2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { cn } from "@/lib/utils";

export function MobileListingMenuButton({
  workspaceId,
  open,
  onClick,
}: {
  workspaceId?: string;
  open: boolean;
  onClick: MouseEventHandler<HTMLButtonElement>;
}) {
  const { t } = useTranslation();
  const { issueSeverity } = useAppStatusDrawer();
  const issueDetails = useConnectionIssueCopy(issueSeverity);
  const { activity, label } = useQuickChatActivity(workspaceId);

  return (
    <Button
      variant="ghost"
      size="icon-lg"
      onClick={onClick}
      className={cn(
        "relative !size-11 shrink-0 cursor-pointer",
        issueSeverity === "lost" && "text-destructive",
        issueSeverity === "unstable" && "text-amber-500",
      )}
      aria-label={
        issueDetails
          ? t("kanban:openMenuWithStatus", { description: issueDetails.description })
          : t("kanban:openMenu")
      }
      aria-description={activity ? label : undefined}
      aria-haspopup="dialog"
      aria-expanded={open}
      data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
      data-testid="mobile-topbar-menu"
    >
      <IconMenu2 className="h-4 w-4" />
      {issueDetails && (
        <span
          className={cn(
            "absolute right-1.5 top-1.5 size-2 rounded-full ring-2 ring-background",
            issueDetails.dotClass,
          )}
          aria-hidden="true"
        />
      )}
      <QuickChatActivityIndicator activity={activity} className="right-1.5 bottom-1.5 top-auto" />
    </Button>
  );
}
