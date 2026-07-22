"use client";

import { IconChevronDown, IconGitPullRequest, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@kandev/ui/lib/utils";
import { prIdentitySlug, prTaskKey } from "@/components/github/pr-utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskPR } from "@/lib/types/github";

type ReviewPRSelectorProps = {
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  loading: boolean;
  onSelectPR: (pr: TaskPR) => void;
  className?: string;
  compact?: boolean;
  testIdPrefix?: string;
};

export function ReviewPRSelector({
  prs,
  selectedPR,
  loading,
  onSelectPR,
  className,
  compact = false,
  testIdPrefix = "review-pr-selector",
}: ReviewPRSelectorProps) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  if (prs.length < 2 || !selectedPR) return null;

  const selectedKey = prTaskKey(selectedPR);
  const touchSized = isMobile || !isFinePointer;
  let triggerHeight = "min-h-8";
  if (touchSized) triggerHeight = "min-h-11";
  else if (compact) triggerHeight = "min-h-5";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          data-testid={`${testIdPrefix}-trigger`}
          data-pr-number={selectedPR.pr_number}
          aria-label={`Review pull request ${selectedPR.repo} #${selectedPR.pr_number}`}
          className={cn(
            "max-w-full cursor-pointer gap-1.5 px-2 transition-transform active:scale-[0.96]",
            triggerHeight,
            className,
          )}
        >
          {loading ? (
            <IconLoader2 className="h-4 w-4 shrink-0 animate-spin" />
          ) : (
            <IconGitPullRequest className="h-4 w-4 shrink-0" />
          )}
          <span className="truncate text-xs font-medium">
            {selectedPR.repo} <span className="tabular-nums">#{selectedPR.pr_number}</span>
          </span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        data-testid={`${testIdPrefix}-menu`}
        className="w-80 max-w-[calc(100vw-1rem)] overflow-y-auto overscroll-contain sm:max-h-80"
      >
        <DropdownMenuLabel>Review pull request</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuRadioGroup
          value={selectedKey}
          onValueChange={(nextKey) => {
            const nextPR = prs.find((pr) => prTaskKey(pr) === nextKey);
            if (nextPR) onSelectPR(nextPR);
          }}
        >
          {prs.map((pr) => (
            <DropdownMenuRadioItem
              key={prTaskKey(pr)}
              value={prTaskKey(pr)}
              data-testid={`${testIdPrefix}-item-${prIdentitySlug(pr)}`}
              data-pr-number={pr.pr_number}
              className="min-h-11 cursor-pointer gap-2 py-2.5 pr-8"
            >
              <IconGitPullRequest className="h-4 w-4 shrink-0" />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-sm font-medium">
                  {pr.repo} <span className="tabular-nums">#{pr.pr_number}</span>
                </span>
                <span className="truncate text-xs text-muted-foreground">{pr.pr_title}</span>
              </span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
