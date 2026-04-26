"use client";

import { useState } from "react";
import { IconChevronRight, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";

type SidebarCollapsibleSectionProps = {
  label: string;
  children: React.ReactNode;
  onAdd?: () => void;
  defaultOpen?: boolean;
};

export function SidebarCollapsibleSection({
  label,
  children,
  onAdd,
  defaultOpen = true,
}: SidebarCollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="flex items-center justify-between px-3 py-1.5">
        <CollapsibleTrigger className="flex items-center gap-1 cursor-pointer">
          <IconChevronRight
            className={cn("h-3 w-3 text-muted-foreground/60 transition-transform", open && "rotate-90")}
          />
          <span className="text-[10px] font-medium uppercase tracking-widest font-mono text-muted-foreground/60">
            {label}
          </span>
        </CollapsibleTrigger>
        {onAdd && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-5 w-5 cursor-pointer"
                onClick={onAdd}
              >
                <IconPlus className="h-3 w-3 text-muted-foreground/60" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Add {label.toLowerCase()}</TooltipContent>
          </Tooltip>
        )}
      </div>
      <CollapsibleContent>
        <div className="flex flex-col gap-0.5">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}
