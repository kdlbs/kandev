"use client";

import { useState } from "react";
import { IconBrandSentry } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { useAppStore } from "@/components/state-provider";
import { SentryIssueDialog } from "./sentry-issue-dialog";

// SentryIssueButton opens the browse/search dialog. It renders nothing when
// the Sentry integration is not available (toggle off or unauthenticated).
export function SentryIssueButton() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const available = useSentryAvailable(workspaceId);
  const [open, setOpen] = useState(false);

  if (!available) return null;

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer px-2 gap-1"
            onClick={() => setOpen(true)}
          >
            <IconBrandSentry className="h-4 w-4" />
            <span className="text-xs font-medium">Sentry</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Browse Sentry issues</TooltipContent>
      </Tooltip>
      <SentryIssueDialog open={open} onOpenChange={setOpen} workspaceId={workspaceId ?? ""} />
    </>
  );
}
