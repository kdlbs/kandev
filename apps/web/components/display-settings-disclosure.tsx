"use client";

import { useId, useState, type ReactNode } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";

export type DisplaySettingsDisclosureProps = {
  title: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
  testId: string;
  defaultExpanded?: boolean;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  className?: string;
  contentClassName?: string;
  /** Use the larger header target when the disclosure is rendered in a phone surface. */
  touchTargets?: boolean;
  summaryClassName?: string;
};

/** A keyboard-accessible disclosure for grouped display settings. */
export function DisplaySettingsDisclosure({
  title,
  summary,
  children,
  testId,
  defaultExpanded = false,
  expanded: controlledExpanded,
  onExpandedChange,
  className,
  contentClassName,
  touchTargets = false,
  summaryClassName,
}: DisplaySettingsDisclosureProps) {
  const generatedId = useId();
  const contentId = `${testId}-content-${generatedId.replace(/:/g, "")}`;
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(defaultExpanded);
  const expanded = controlledExpanded ?? uncontrolledExpanded;
  const setExpanded = (next: boolean) => {
    if (controlledExpanded === undefined) setUncontrolledExpanded(next);
    onExpandedChange?.(next);
  };

  return (
    <section className={cn("px-2 pb-1 pt-1", className)} data-testid={testId}>
      <Button
        type="button"
        variant="ghost"
        className={cn(
          "!h-auto flex min-h-7 w-full items-center justify-between gap-2 px-1 py-1 text-left [@media(pointer:coarse)]:min-h-11",
          touchTargets && "min-h-11",
        )}
        aria-expanded={expanded}
        aria-controls={contentId}
        data-testid={`${testId}-toggle`}
        onClick={() => setExpanded(!expanded)}
      >
        <span className="min-w-0 flex-1 pt-1">
          <span className="block text-[11px] font-medium uppercase leading-none tracking-wide text-muted-foreground">
            {title}
          </span>
          {summary && (
            <span
              className={cn(
                "mt-1 block whitespace-normal break-words text-xs text-muted-foreground",
                summaryClassName,
              )}
            >
              {summary}
            </span>
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
