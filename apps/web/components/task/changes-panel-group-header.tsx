"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

type ChangesPanelGroupHeaderProps = Omit<
  ComponentPropsWithoutRef<"button">,
  "aria-expanded" | "children" | "type"
> & {
  label: string;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
  testId: string;
  touchMode?: boolean;
  id?: string;
};

export const ChangesPanelGroupHeader = forwardRef<HTMLButtonElement, ChangesPanelGroupHeaderProps>(
  function ChangesPanelGroupHeader(
    {
      label,
      count,
      collapsed,
      onToggle,
      testId,
      touchMode = false,
      id,
      className,
      onClick,
      ...buttonProps
    },
    ref,
  ) {
    return (
      <button
        {...buttonProps}
        ref={ref}
        id={id}
        type="button"
        className={cn(
          "flex min-h-6 min-w-0 items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground/80 cursor-pointer hover:text-foreground/80 [@media(pointer:coarse)]:min-h-11",
          touchMode && "min-h-11",
          className,
        )}
        data-testid={testId}
        data-changes-row-focus
        aria-expanded={!collapsed}
        onClick={(event) => {
          onClick?.(event);
          onToggle();
        }}
      >
        {collapsed ? (
          <IconChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/50" />
        ) : (
          <IconChevronDown className="h-3 w-3 shrink-0 text-muted-foreground/50" />
        )}
        <span className="truncate">{label}</span>
        <span className="text-muted-foreground/50 normal-case tracking-normal">{count}</span>
      </button>
    );
  },
);
