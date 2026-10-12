"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function ConfigurationChatAction({
  checked,
  disabled,
  onCheckedChange,
}: {
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const label = t("chat:configurationChat");
  const description = t("chat:configurationChatDescription");
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  const usesSheet = touch || isMobile;
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant={checked ? "default" : "ghost"}
      size="icon"
      role={usesSheet ? undefined : "switch"}
      aria-checked={usesSheet ? undefined : checked}
      aria-label={label}
      aria-describedby="quick-chat-configuration-help"
      aria-haspopup={usesSheet ? "dialog" : undefined}
      aria-expanded={usesSheet ? open : undefined}
      disabled={disabled}
      data-testid="quick-chat-configuration-action"
      className={usesSheet ? "h-11 w-11 shrink-0" : "h-7 w-7 shrink-0"}
      onClick={() => (usesSheet ? setOpen(true) : onCheckedChange(!checked))}
    >
      <IconSettings className="h-4 w-4" aria-hidden />
    </Button>
  );
  return (
    <>
      <span id="quick-chat-configuration-help" className="sr-only">
        {description}
      </span>
      {usesSheet ? (
        <>
          {trigger}
          <Drawer open={open} onOpenChange={setOpen}>
            <DrawerContent
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                triggerRef.current?.focus();
              }}
            >
              <DrawerHeader>
                <DrawerTitle>{label}</DrawerTitle>
                <DrawerDescription>{description}</DrawerDescription>
              </DrawerHeader>
              <label className="flex min-h-12 cursor-pointer items-center justify-between gap-4 px-4 pb-4">
                {label}
                <Switch
                  checked={checked}
                  disabled={disabled}
                  onCheckedChange={(next) => {
                    onCheckedChange(next);
                    setOpen(false);
                  }}
                />
              </label>
            </DrawerContent>
          </Drawer>
        </>
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
          <TooltipContent className="max-w-xs">
            <p className="font-medium">{label}</p>
            <p>{description}</p>
          </TooltipContent>
        </Tooltip>
      )}
    </>
  );
}
