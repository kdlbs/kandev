import type { ComponentProps } from "react";
import { ContextMenuPortal, ContextMenuSubContent } from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";

/** Portal each level so a positioned parent cannot become a submenu's containing block. */
export function TaskContextMenuSubContent({
  className,
  ...props
}: ComponentProps<typeof ContextMenuSubContent>) {
  return (
    <ContextMenuPortal>
      <ContextMenuSubContent
        {...props}
        className={cn(
          "max-h-(--radix-context-menu-content-available-height) max-w-[calc(100vw-16px)] overflow-x-hidden overflow-y-auto overscroll-contain",
          className,
        )}
      />
    </ContextMenuPortal>
  );
}
