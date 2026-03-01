"use client";

import { useRouter } from "next/navigation";
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
import { useAppStore } from "@/components/state-provider";
import type { HealthIssue } from "@/lib/types/health";

type HealthIndicatorButtonProps = {
  hasIssues: boolean;
  onClick: () => void;
};

export function HealthIndicatorButton({ hasIssues, onClick }: HealthIndicatorButtonProps) {
  if (!hasIssues) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="icon" onClick={onClick} className="cursor-pointer relative">
          <IconAlertTriangle className="h-4 w-4 text-amber-500" />
          <span className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-amber-500 border-2 border-background" />
          <span className="sr-only">Setup Issues</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Setup Issues</TooltipContent>
    </Tooltip>
  );
}

type HealthIssuesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  issues: HealthIssue[];
};

export function HealthIssuesDialog({ open, onOpenChange, issues }: HealthIssuesDialogProps) {
  const router = useRouter();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);

  const resolveUrl = (url: string) => url.replace("{workspaceId}", workspaceId ?? "");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconAlertTriangle className="h-5 w-5 text-amber-500" />
            Setup Issues
          </DialogTitle>
          <DialogDescription>
            {issues.length === 1
              ? "1 issue needs your attention"
              : `${issues.length} issues need your attention`}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 pt-2">
          {issues.map((issue) => (
            <div key={issue.id} className="rounded-lg border p-3 space-y-2">
              <div className="font-medium text-sm">{issue.title}</div>
              <div className="text-muted-foreground text-xs">{issue.message}</div>
              <Button
                variant="outline"
                size="sm"
                className="cursor-pointer h-7 text-xs"
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
