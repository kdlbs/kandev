"use client";

import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";

export const USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS =
  "pr-[calc(4rem+env(safe-area-inset-right))]";

export function usePersistentUserMessageNavigationRail() {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  return isMobile || !isFinePointer;
}

export type UserMessageNavigationRailProps = {
  canNavigatePrevious: boolean;
  canNavigateNext: boolean;
  isBusy: boolean;
  onPrevious: () => void;
  onNext: () => void;
  className?: string;
};

export function UserMessageNavigationRail({
  canNavigatePrevious,
  canNavigateNext,
  isBusy,
  onPrevious,
  onNext,
  className,
}: UserMessageNavigationRailProps) {
  const usesPersistentControls = usePersistentUserMessageNavigationRail();
  const buttonClassName = usesPersistentControls ? "h-11 w-11" : "h-8 w-8";

  return (
    <nav
      aria-label="User message navigation"
      aria-busy={isBusy}
      data-testid="user-message-navigation-rail"
      className={cn(
        "absolute right-[calc(0.5rem+env(safe-area-inset-right))] top-1/2 z-20 flex -translate-y-1/2 flex-col overflow-hidden rounded-md border border-border/70 bg-background/90 shadow-sm backdrop-blur-sm transition-opacity",
        usesPersistentControls
          ? "pointer-events-auto opacity-100"
          : "pointer-events-none opacity-0 group-hover/chat:pointer-events-auto group-hover/chat:opacity-100 group-focus-within/chat:pointer-events-auto group-focus-within/chat:opacity-100",
        className,
      )}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Previous user message"
        title="Previous user message"
        data-testid="previous-user-message"
        disabled={isBusy || !canNavigatePrevious}
        onClick={onPrevious}
        className={cn(
          "cursor-pointer rounded-none border-b border-border/70 text-muted-foreground hover:text-foreground",
          buttonClassName,
        )}
      >
        <IconChevronUp aria-hidden />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Next user message"
        title="Next user message"
        data-testid="next-user-message"
        disabled={isBusy || !canNavigateNext}
        onClick={onNext}
        className={cn(
          "cursor-pointer rounded-none text-muted-foreground hover:text-foreground",
          buttonClassName,
        )}
      >
        <IconChevronDown aria-hidden />
      </Button>
    </nav>
  );
}
