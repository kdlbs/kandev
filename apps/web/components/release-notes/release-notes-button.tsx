"use client";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconSparkles } from "@tabler/icons-react";

type ReleaseNotesButtonProps = {
  hasUnseen: boolean;
  onClick: () => void;
};

export function ReleaseNotesButton({ hasUnseen, onClick }: ReleaseNotesButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="icon" onClick={onClick} className="cursor-pointer relative">
          <IconSparkles className="h-4 w-4" />
          {hasUnseen && (
            <span className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-primary border-2 border-background" />
          )}
          <span className="sr-only">What&apos;s New</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>What&apos;s New</TooltipContent>
    </Tooltip>
  );
}
