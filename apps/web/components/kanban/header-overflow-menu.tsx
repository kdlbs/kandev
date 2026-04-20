"use client";

import Link from "next/link";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { IconDots, IconRefresh, IconSettings, IconSparkles } from "@tabler/icons-react";
import { useRefreshReviews } from "@/hooks/use-refresh-reviews";

type HeaderOverflowMenuProps = {
  showReleaseNotes: boolean;
  onOpenReleaseNotes: () => void;
};

export function HeaderOverflowMenu({
  showReleaseNotes,
  onOpenReleaseNotes,
}: HeaderOverflowMenuProps) {
  const { available: refreshAvailable, loading: refreshLoading, trigger: triggerRefresh } =
    useRefreshReviews();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="icon" className="cursor-pointer relative">
          <IconDots className="h-4 w-4" />
          {showReleaseNotes && (
            <span className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-primary border-2 border-background" />
          )}
          <span className="sr-only">More</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {refreshAvailable && (
          <DropdownMenuItem
            onSelect={triggerRefresh}
            disabled={refreshLoading}
            className="cursor-pointer"
          >
            <IconRefresh className={`h-4 w-4 mr-2 ${refreshLoading ? "animate-spin" : ""}`} />
            Check for PRs to review
          </DropdownMenuItem>
        )}
        {showReleaseNotes && (
          <DropdownMenuItem onSelect={onOpenReleaseNotes} className="cursor-pointer">
            <IconSparkles className="h-4 w-4 mr-2" />
            What&apos;s New
          </DropdownMenuItem>
        )}
        <DropdownMenuItem asChild className="cursor-pointer">
          <Link href="/settings">
            <IconSettings className="h-4 w-4 mr-2" />
            Settings
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
