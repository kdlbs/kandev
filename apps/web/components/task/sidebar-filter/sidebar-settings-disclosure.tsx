"use client";

import { useId, useState, type ReactNode } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";

type Props = {
  title: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
  testId: string;
  defaultExpanded?: boolean;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  className?: string;
  contentClassName?: string;
};

/** Shared compact/mobile-friendly disclosure used by sidebar view settings. */
export function SidebarSettingsDisclosure({
  title,
  summary,
  children,
  testId,
  defaultExpanded = false,
  expanded: controlledExpanded,
  onExpandedChange,
  className,
  contentClassName,
}: Props) {
  const generatedId = useId();
  const contentId = `${testId}-content-${generatedId.replace(/:/g, "")}`;
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(defaultExpanded);
  const expanded = controlledExpanded ?? uncontrolledExpanded;
  const setExpanded = (next: boolean) => {
    if (controlledExpanded === undefined) setUncontrolledExpanded(next);
    onExpandedChange?.(next);
  };

  return (
    <section className={cn("px-2 pb-2 pt-2", className)} data-testid={testId}>
      <Button
        type="button"
        variant="ghost"
        className="flex min-h-11 w-full items-center justify-between px-1 text-left"
        aria-expanded={expanded}
        aria-controls={contentId}
        data-testid={`${testId}-toggle`}
        onClick={() => setExpanded(!expanded)}
      >
        <span className="min-w-0 pt-1">
          <span className="block text-[11px] font-medium uppercase leading-none tracking-wide text-muted-foreground">
            {title}
          </span>
          {summary && (
            <span className="mt-1 block truncate text-xs text-muted-foreground">{summary}</span>
          )}
        </span>
        <IconChevronDown
          className={cn(
            "size-4 shrink-0 text-muted-foreground transition-transform",
            expanded && "rotate-180",
          )}
          aria-hidden="true"
        />
      </Button>
      {expanded && (
        <div id={contentId} className={contentClassName}>
          {children}
        </div>
      )}
    </section>
  );
}
