"use client";

import type { ComponentProps } from "react";
import { Button } from "@kandev/ui/button";
import { IconChevronDown } from "@tabler/icons-react";

export function MobileListingContext({
  context,
  label,
  ...props
}: ComponentProps<typeof Button> & { context: string; label: string }) {
  return (
    <Button
      {...props}
      type="button"
      variant="ghost"
      className="h-auto min-h-11 min-w-0 max-w-full shrink cursor-pointer justify-start px-1 py-1"
    >
      <span className="flex min-w-0 flex-col text-left">
        <span className="truncate text-xs font-normal leading-4 text-muted-foreground">
          {context}
        </span>
        <span className="flex min-w-0 items-center gap-1.5 text-sm leading-5">
          <span className="truncate">{label}</span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        </span>
      </span>
    </Button>
  );
}
