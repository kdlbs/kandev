"use client";

import { useState } from "react";
import { IconBug } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { SentryIssueDialog } from "./sentry-issue-dialog";

// SentryIssueButton opens the browse/search dialog. It renders nothing when
// the Sentry integration is not available (toggle off or unauthenticated).
export function SentryIssueButton() {
  const available = useSentryAvailable();
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
            <IconBug className="h-4 w-4" />
            <span className="text-xs font-medium">Sentry</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Browse Sentry issues</TooltipContent>
      </Tooltip>
      <SentryIssueDialog open={open} onOpenChange={setOpen} />
    </>
  );
}
