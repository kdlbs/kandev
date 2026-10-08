"use client";

import { useState, type ReactNode } from "react";
import { IconChevronRight } from "@tabler/icons-react";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@kandev/ui/collapsible";
import { cn } from "@/lib/utils";
import type { StepSection } from "@/lib/workflows/workflow-step-section-summary";

export function StepSummarySection({
  section,
  label,
  summary,
  children,
  dirty,
  keepMounted,
  defaultOpen = false,
}: {
  section: StepSection;
  label: string;
  summary: string;
  children: ReactNode;
  dirty: boolean;
  keepMounted?: boolean;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="group flex min-h-11 w-full cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring md:items-center"
          data-testid={`workflow-section-toggle-${section}`}
          data-settings-dirty={dirty}
          data-settings-dirty-level="container"
        >
          <IconChevronRight
            className={cn(
              "mt-0.5 h-4 w-4 shrink-0 text-muted-foreground transition-transform md:mt-0",
              open && "rotate-90",
            )}
            aria-hidden
          />
          <span className="flex min-w-0 flex-1 flex-col gap-1 md:flex-row md:items-baseline md:gap-5">
            <span className="shrink-0 text-sm font-medium md:w-28">{label}</span>
            <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground" title={summary}>
              {summary}
            </span>
          </span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent
        forceMount={keepMounted ? true : undefined}
        hidden={!open}
        className="min-w-0 px-4 pb-4 md:pl-11"
        data-testid={`workflow-section-content-${section}`}
      >
        {children}
      </CollapsibleContent>
    </Collapsible>
  );
}
