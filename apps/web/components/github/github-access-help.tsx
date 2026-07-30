"use client";

import { useState } from "react";
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
}: {
  label: string;
  title: string;
  description: string;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-11 shrink-0 cursor-pointer text-muted-foreground sm:h-6 sm:w-6"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={label}
    >
      <IconInfoCircle className="h-4 w-4" />
    </Button>
  );
  const drawerTrigger = <DrawerTrigger asChild>{button}</DrawerTrigger>;
  const trigger = usesTouchDrawer ? (
    drawerTrigger
  ) : (
    <Tooltip>
      <TooltipTrigger asChild>{drawerTrigger}</TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-[320px] text-xs leading-relaxed">
        {description}
      </TooltipContent>
    </Tooltip>
  );

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{title}</DrawerTitle>
          <DrawerDescription>{description}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}
