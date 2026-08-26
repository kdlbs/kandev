"use client";

import { useRouter } from "@/lib/routing/client-router";
import type { ComponentProps } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@kandev/ui/dialog";
import { IconAlertTriangle, IconExternalLink } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import type { HealthIssue } from "@/lib/types/health";

type HealthIndicatorButtonProps = {
  hasIssues: boolean;
  onClick: () => void;
  size?: ComponentProps<typeof Button>["size"];
};

export function HealthIndicatorButton({
  hasIssues,
  onClick,
  size = "icon",
}: HealthIndicatorButtonProps) {
  const { t } = useTranslation();
  if (!hasIssues) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size={size} onClick={onClick} className="cursor-pointer relative">
          <IconAlertTriangle className="h-4 w-4 text-amber-500" />
          <span className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-amber-500 border-2 border-background" />
          <span className="sr-only">{t("system:setupIssues")}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("system:setupIssues")}</TooltipContent>
    </Tooltip>
  );
}

type HealthIssuesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  issues: HealthIssue[];
};

export function HealthIssuesDialog({ open, onOpenChange, issues }: HealthIssuesDialogProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);

  const resolveUrl = (url: string) => url.replace("{workspaceId}", workspaceId ?? "");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="system-health-issues-dialog"
        data-layout="contained"
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-[480px]"
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconAlertTriangle className="h-5 w-5 text-amber-500" />
            {t("system:setupIssues")}
          </DialogTitle>
          <DialogDescription>
            {t("system:issuesNeedAttention", { count: issues.length })}
          </DialogDescription>
        </DialogHeader>
        <div
          data-testid="system-health-issues-body"
          className="min-h-0 min-w-0 space-y-4 overflow-x-hidden overflow-y-auto overscroll-contain pt-2"
        >
          {issues.map((issue) => (
            <div
              key={issue.id}
              data-testid={`system-health-issue-${issue.id}`}
              className="rounded-lg border p-3 space-y-2"
            >
              <div className="font-medium text-sm">{issue.title}</div>
              <div className="text-muted-foreground text-xs">{issue.message}</div>
              <Button
                variant="outline"
                size="sm"
                className="min-h-11 cursor-pointer h-7 text-xs sm:min-h-0"
                onClick={() => {
                  onOpenChange(false);
                  router.push(resolveUrl(issue.fix_url));
                }}
              >
                {issue.fix_label}
                <IconExternalLink className="h-3 w-3 ml-1" />
              </Button>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
