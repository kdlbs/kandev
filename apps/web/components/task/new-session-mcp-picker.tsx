"use client";

import { useId, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { SettingsInfo } from "@/components/settings/settings-info";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isHandledApiError } from "@/lib/api/client";
import { MCPSessionSelectionBody, type MCPSessionSelectorProps } from "./mcp-session-selector";

export type NewSessionMCPPickerProps = MCPSessionSelectorProps & {
  portalContainer: HTMLElement | null;
};

function containPickerEnter(event: KeyboardEvent) {
  if (event.key !== "Enter" || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229)
    return;
  event.stopPropagation();
  // Search must not invoke the launch dialog's default action or submit its form.
  if (event.target instanceof HTMLInputElement) event.preventDefault();
}

export function NewSessionMCPPicker({ portalContainer, ...selection }: NewSessionMCPPickerProps) {
  const { t } = useTranslation();
  const id = useId();
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const title = t("settings:mcpServers");
  const description = t("settings:mcpSessionSelectionDescription");
  const unavailable = isHandledApiError(selection.error);
  const trigger = (
    <Button
      id={id}
      type="button"
      variant="outline"
      aria-labelledby={`${id}-label ${id}-count`}
      aria-haspopup="dialog"
      aria-expanded={open && !unavailable}
      data-testid="new-session-mcp-select"
      disabled={selection.disabled || unavailable}
      className={controlSizingClassName(
        "standard",
        "w-32 max-w-full justify-between border-border/60 bg-muted/30 hover:bg-muted/60 cursor-pointer",
      )}
    >
      <span id={`${id}-count`} className="min-w-0 truncate tabular-nums">
        {t("settings:mcpSelectedCount", { count: selection.selectedIds.length })}
      </span>
      <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
    </Button>
  );
  const body = (
    <MCPSessionSelectionBody
      {...selection}
      testId="new-session-mcp"
      query={query}
      onQueryChange={setQuery}
    />
  );
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2" data-testid="new-session-mcp-row">
      <div className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
        <label id={`${id}-label`} htmlFor={id} className="wrap-anywhere">
          {title}
        </label>
        <SettingsInfo label={title}>{description}</SettingsInfo>
      </div>
      {touch || isMobile ? (
        <Drawer open={open && !unavailable} onOpenChange={setOpen}>
          <DrawerTrigger asChild>{trigger}</DrawerTrigger>
          <DrawerContent
            data-testid="new-session-mcp-options"
            className="h-[min(80dvh,42rem)] !max-h-[80dvh] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]"
            onKeyDown={containPickerEnter}
          >
            <DrawerHeader className="shrink-0 text-left">
              <div className="flex min-w-0 items-center justify-between gap-3">
                <DrawerTitle>{title}</DrawerTitle>
                <DrawerClose asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className={controlSizingClassName("icon", "cursor-pointer")}
                    aria-label={t("common:close")}
                  >
                    <IconX className="size-4" aria-hidden="true" />
                  </Button>
                </DrawerClose>
              </div>
              <DrawerDescription>{description}</DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-4">
              {body}
            </div>
          </DrawerContent>
        </Drawer>
      ) : (
        <Popover open={open && !unavailable} onOpenChange={setOpen}>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent
            align="start"
            portalContainer={portalContainer}
            aria-label={title}
            data-testid="new-session-mcp-options"
            className="max-h-[min(28rem,var(--radix-popover-content-available-height))] w-[min(24rem,calc(100vw-2rem))] gap-3 overflow-hidden p-3"
            onKeyDown={containPickerEnter}
          >
            <p className="shrink-0 text-xs text-pretty text-muted-foreground">{description}</p>
            <div className="min-h-0 overflow-y-auto overscroll-contain">{body}</div>
          </PopoverContent>
        </Popover>
      )}
    </div>
  );
}
