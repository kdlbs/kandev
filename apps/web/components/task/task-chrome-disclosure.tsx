"use client";

import { useRef, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

/** Task chrome uses click/focus disclosures on desktop and native drawers on touch. */
export function TaskChromeDisclosure({
  label,
  icon,
  children,
  testId,
  showLabel = false,
  open: controlledOpen,
  onOpenChange,
}: {
  label: string;
  icon: ReactNode;
  children: ReactNode;
  testId: string;
  showLabel?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlledOpen ?? internalOpen;
  const setOpen = onOpenChange ?? setInternalOpen;
  const contentRef = useRef<HTMLDivElement>(null);
  const touch = useTouchDrawer();
  const trigger = (
    <Button
      variant="ghost"
      size={showLabel ? "sm" : "icon"}
      className="h-7 cursor-pointer gap-1.5 text-xs text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
      aria-label={label}
      title={label}
      data-testid={testId}
    >
      {icon}
      {showLabel && <span className="hidden xl:inline">{label}</span>}
    </Button>
  );

  if (touch) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[80dvh]" aria-describedby={undefined}>
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{label}</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto overscroll-contain px-4 pb-[calc(1rem+env(safe-area-inset-bottom))] [&_button]:min-h-11 [&_button]:min-w-11">
            {children}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        ref={contentRef}
        role="dialog"
        aria-label={label}
        align="end"
        className="w-80 max-w-[calc(100vw-1rem)] space-y-3 p-3"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          contentRef.current?.focus();
        }}
      >
        <h2 className="text-xs font-medium">{label}</h2>
        {children}
      </PopoverContent>
    </Popover>
  );
}
