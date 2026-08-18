"use client";

import { useState, type ReactNode } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

export function GitHubAccessHelp({
  label,
  title,
  description,
  content,
  icon,
  onOpen,
}: {
  label: string;
  title: string;
  description: string;
  content?: ReactNode;
  icon?: ReactNode;
  onOpen?: () => void;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const handleDrawerOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (nextOpen) onOpen?.();
  };
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-11 shrink-0 cursor-pointer text-muted-foreground sm:h-6 sm:w-6"
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
      aria-label={label}
    >
      {icon ?? <IconInfoCircle className="h-4 w-4" />}
    </Button>
  );
  const trigger = usesTouchDrawer ? (
    <DrawerTrigger asChild>{button}</DrawerTrigger>
  ) : (
    <Tooltip onOpenChange={(nextOpen) => nextOpen && onOpen?.()}>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-[320px] text-xs leading-relaxed">
        {content ?? description}
      </TooltipContent>
    </Tooltip>
  );

  return (
    <Drawer open={open} onOpenChange={handleDrawerOpenChange}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{title}</DrawerTitle>
          <DrawerDescription className={content ? "sr-only" : undefined}>
            {description}
          </DrawerDescription>
        </DrawerHeader>
        {content && <div className="px-4 pb-4">{content}</div>}
      </DrawerContent>
    </Drawer>
  );
}
