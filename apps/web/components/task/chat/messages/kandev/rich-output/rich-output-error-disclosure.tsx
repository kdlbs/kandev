"use client";

import { useId, useRef, useState, type ReactNode, type RefObject } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerClose, DrawerContent, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTranslation } from "react-i18next";
import { getRichOutputErrorSummary } from "./rich-output-error";

type DisclosureSurfaceProps = {
  diagnostic: string;
  title: string;
  titleId: string;
  diagnosticId: string;
  dialogId: string;
  open: boolean;
  setOpen: (open: boolean) => void;
  trigger: ReactNode;
  triggerRef: RefObject<HTMLButtonElement | null>;
};

export function RichOutputErrorDisclosure({ diagnostic }: { diagnostic: string }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const usesTouchDrawerOnPointer = useTouchDrawer();
  const usesDrawer = isMobile || usesTouchDrawerOnPointer;
  const [open, setOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const id = useId();
  const titleId = `${id}-title`;
  const diagnosticId = `${id}-diagnostic`;
  const title = t("task:richOutputErrorTitle");
  const trigger = (
    <button
      ref={triggerRef}
      type="button"
      className={`inline-flex shrink-0 cursor-pointer items-center justify-center rounded text-red-500 hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${usesDrawer ? "size-11" : "size-6"}`}
      aria-label={t("task:richOutputErrorTrigger")}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-controls={open ? `${id}-dialog` : undefined}
      data-testid="rich-output-error-trigger"
    >
      <IconX aria-hidden="true" className="size-3.5" />
    </button>
  );
  const surfaceProps = {
    diagnostic,
    title,
    titleId,
    diagnosticId,
    dialogId: `${id}-dialog`,
    open,
    setOpen,
    trigger,
    triggerRef,
  };

  return usesDrawer ? (
    <MobileErrorDrawer {...surfaceProps} />
  ) : (
    <DesktopErrorPopover
      {...surfaceProps}
      tooltipOpen={tooltipOpen}
      setTooltipOpen={setTooltipOpen}
    />
  );
}

function MobileErrorDrawer({
  diagnostic,
  title,
  diagnosticId,
  dialogId,
  open,
  setOpen,
  trigger,
}: DisclosureSurfaceProps) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={setOpen} shouldScaleBackground={false}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent
        id={dialogId}
        aria-describedby={diagnosticId}
        data-testid="rich-output-error-drawer"
        className="max-h-[min(36rem,calc(100dvh-1rem))] overflow-hidden pb-[env(safe-area-inset-bottom,0px)]"
      >
        <div className="flex shrink-0 items-center justify-between gap-3 border-b px-4 py-2 text-left">
          <DrawerTitle className="min-w-0 break-words text-sm">{title}</DrawerTitle>
          <DrawerClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-11 shrink-0"
              aria-label={t("common:close")}
            >
              <IconX aria-hidden="true" className="size-4" />
            </Button>
          </DrawerClose>
        </div>
        <div
          id={diagnosticId}
          data-testid="rich-output-error-scroll-region"
          className="min-h-0 overflow-y-auto overscroll-contain break-words whitespace-pre-wrap [overflow-wrap:anywhere] p-4"
          data-vaul-no-drag
        >
          {diagnostic}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DesktopErrorPopover({
  diagnostic,
  title,
  titleId,
  diagnosticId,
  dialogId,
  open,
  setOpen,
  trigger,
  triggerRef,
  tooltipOpen,
  setTooltipOpen,
}: DisclosureSurfaceProps & {
  tooltipOpen: boolean;
  setTooltipOpen: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const summary = getRichOutputErrorSummary(diagnostic);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip open={!open && tooltipOpen} onOpenChange={setTooltipOpen}>
        <PopoverTrigger asChild>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
        </PopoverTrigger>
        <TooltipContent data-testid="rich-output-error-tooltip" side="top" sideOffset={6}>
          {summary}
        </TooltipContent>
      </Tooltip>
      <PopoverContent
        id={dialogId}
        role="dialog"
        aria-labelledby={titleId}
        aria-describedby={diagnosticId}
        data-testid="rich-output-error-popover"
        align="end"
        side="bottom"
        sideOffset={6}
        className="max-h-[min(36rem,calc(100dvh-2rem))] overflow-hidden p-0"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus();
        }}
      >
        <div className="flex shrink-0 items-center justify-between gap-3 border-b px-3 py-2">
          <h2 id={titleId} className="min-w-0 break-words text-sm font-medium">
            {title}
          </h2>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 shrink-0"
            aria-label={t("common:close")}
            onClick={() => setOpen(false)}
          >
            <IconX aria-hidden="true" className="size-4" />
          </Button>
        </div>
        <div
          id={diagnosticId}
          data-testid="rich-output-error-scroll-region"
          className="min-h-0 overflow-y-auto overscroll-contain break-words whitespace-pre-wrap [overflow-wrap:anywhere] p-3"
        >
          {diagnostic}
        </div>
      </PopoverContent>
    </Popover>
  );
}
