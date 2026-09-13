"use client";

import {
  forwardRef,
  useEffect,
  useId,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type ForwardedRef,
  type ReactNode,
} from "react";
import { IconTarget, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentGoal } from "@/lib/agent-goal";
import { cn } from "@/lib/utils";

type AgentGoalDetailsProps = {
  goal: AgentGoal;
  drawer?: boolean;
};

function AgentGoalDetails({ goal, drawer = false }: AgentGoalDetailsProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="agent-goal-details"
      className={cn("space-y-2", drawer && "min-h-0 overflow-y-auto p-4")}
      data-vaul-no-drag={drawer ? true : undefined}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-foreground">{t("task:goalDetailsTitle")}</span>
        <span className="shrink-0 text-[11px] text-primary">{t("task:goalStatusActive")}</span>
      </div>
      <p
        data-testid="agent-goal-objective"
        className="max-h-48 overflow-y-auto whitespace-pre-wrap text-foreground [overflow-wrap:anywhere]"
      >
        {goal.objective}
      </p>
      <p className="text-muted-foreground">{t("task:goalContinuationExplanation")}</p>
    </div>
  );
}

export type AgentGoalChipProps = {
  goal: AgentGoal | null;
};

type AgentGoalTriggerProps = {
  detailsId: string;
  open: boolean;
  restoringFocusRef: { current: boolean };
  setOpen: (open: boolean) => void;
  suppressHoverRef: { current: boolean };
  triggerRef: { current: HTMLButtonElement | null };
  triggerWasFocusedRef: { current: boolean };
  onHoverChange: (hovered: boolean) => void;
  usesDrawer: boolean;
} & Omit<ComponentPropsWithoutRef<"button">, "ref">;

const AgentGoalTrigger = forwardRef<HTMLButtonElement, AgentGoalTriggerProps>(
  function AgentGoalTrigger(
    {
      detailsId,
      open,
      restoringFocusRef,
      setOpen,
      suppressHoverRef,
      triggerRef,
      triggerWasFocusedRef,
      onHoverChange,
      usesDrawer,
      ...buttonProps
    },
    forwardedRef: ForwardedRef<HTMLButtonElement>,
  ) {
    const { t } = useTranslation();
    return (
      <button
        {...buttonProps}
        type="button"
        ref={(node) => {
          triggerRef.current = node;
          if (typeof forwardedRef === "function") forwardedRef(node);
          else if (forwardedRef) forwardedRef.current = node;
        }}
        data-testid="agent-goal-chip"
        aria-label={t("task:goalActive")}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={detailsId}
        onMouseEnter={() => {
          onHoverChange(true);
          if (!usesDrawer && !suppressHoverRef.current) setOpen(true);
        }}
        onMouseLeave={() => {
          onHoverChange(false);
          suppressHoverRef.current = false;
        }}
        onFocus={() => {
          if (!usesDrawer) {
            if (restoringFocusRef.current) {
              restoringFocusRef.current = false;
              return;
            }
            triggerWasFocusedRef.current = true;
            setOpen(true);
          }
        }}
        onKeyDown={(event) => {
          if (!usesDrawer && event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            suppressHoverRef.current = true;
            setOpen(false);
          }
        }}
        className={cn(
          buttonProps.className,
          "inline-flex items-center justify-center gap-1 rounded-full border border-primary/40 bg-primary/10 text-[11px] font-medium text-primary transition-colors hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          usesDrawer ? "min-h-11 min-w-11 px-3" : "h-6 px-2",
        )}
      >
        <IconTarget className="h-3.5 w-3.5" aria-hidden="true" />
        <span>{t("task:goalActive")}</span>
      </button>
    );
  },
);

type AgentGoalPopoverProps = {
  children: ReactNode;
  detailsId: string;
  goal: AgentGoal;
  open: boolean;
  setOpen: (open: boolean) => void;
  suppressHoverRef: { current: boolean };
  triggerRef: { current: HTMLButtonElement | null };
  triggerWasFocusedRef: { current: boolean };
  restoringFocusRef: { current: boolean };
  onHoverChange: (hovered: boolean) => void;
};

function AgentGoalPopover({
  children,
  detailsId,
  goal,
  open,
  setOpen,
  suppressHoverRef,
  triggerRef,
  triggerWasFocusedRef,
  restoringFocusRef,
  onHoverChange,
}: AgentGoalPopoverProps) {
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        id={detailsId}
        data-testid="agent-goal-popover"
        side="top"
        align="start"
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          suppressHoverRef.current = true;
          if (triggerWasFocusedRef.current) {
            restoringFocusRef.current = true;
            triggerRef.current?.focus();
          }
          triggerWasFocusedRef.current = false;
        }}
        onEscapeKeyDown={(event) => {
          event.preventDefault();
          event.stopPropagation();
          suppressHoverRef.current = true;
          setOpen(false);
        }}
        onMouseEnter={() => onHoverChange(true)}
        onMouseLeave={() => onHoverChange(false)}
      >
        <AgentGoalDetails goal={goal} />
      </PopoverContent>
    </Popover>
  );
}

type AgentGoalDrawerProps = {
  children: ReactNode;
  detailsId: string;
  goal: AgentGoal;
  open: boolean;
  setOpen: (open: boolean) => void;
};

function AgentGoalDrawer({ children, detailsId, goal, open, setOpen }: AgentGoalDrawerProps) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{children}</DrawerTrigger>
      <DrawerContent
        id={detailsId}
        data-testid="agent-goal-drawer-content"
        className="max-h-[min(80dvh,calc(100dvh-env(safe-area-inset-bottom)))]"
      >
        <DrawerHeader className="shrink-0 flex-row items-center justify-between gap-3 text-left">
          <div className="min-w-0">
            <DrawerTitle>{t("task:goalDetailsTitle")}</DrawerTitle>
            <DrawerDescription>{t("task:goalDetailsDescription")}</DrawerDescription>
          </div>
          <DrawerClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t("task:goalCloseDetails")}
              className="[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
            >
              <IconX aria-hidden="true" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <AgentGoalDetails goal={goal} drawer />
      </DrawerContent>
    </Drawer>
  );
}

export function AgentGoalChip({ goal }: AgentGoalChipProps) {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesDrawer = !isFinePointer || isMobile;
  const [open, setOpen] = useState(false);
  const suppressHoverRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const triggerWasFocusedRef = useRef(false);
  const restoringFocusRef = useRef(false);
  const triggerHoveredRef = useRef(false);
  const contentHoveredRef = useRef(false);
  const hoverCloseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const detailsId = `agent-goal-details-${useId()}`;
  const isActive = goal?.status === "active";

  useEffect(() => {
    if (!isActive) setOpen(false);
  }, [isActive]);

  useEffect(
    () => () => {
      if (hoverCloseTimerRef.current !== null) clearTimeout(hoverCloseTimerRef.current);
    },
    [],
  );

  if (!isActive || !goal) return null;

  const setHoverPresence = (region: "trigger" | "content", hovered: boolean) => {
    if (region === "trigger") triggerHoveredRef.current = hovered;
    else contentHoveredRef.current = hovered;
    if (hovered) {
      if (hoverCloseTimerRef.current !== null) {
        clearTimeout(hoverCloseTimerRef.current);
        hoverCloseTimerRef.current = null;
      }
      return;
    }
    if (triggerHoveredRef.current || contentHoveredRef.current) return;
    if (hoverCloseTimerRef.current !== null) clearTimeout(hoverCloseTimerRef.current);
    hoverCloseTimerRef.current = setTimeout(() => {
      hoverCloseTimerRef.current = null;
      if (!triggerHoveredRef.current && !contentHoveredRef.current) setOpen(false);
    }, 100);
  };

  const trigger = (
    <AgentGoalTrigger
      detailsId={detailsId}
      open={open}
      restoringFocusRef={restoringFocusRef}
      setOpen={setOpen}
      suppressHoverRef={suppressHoverRef}
      triggerRef={triggerRef}
      triggerWasFocusedRef={triggerWasFocusedRef}
      onHoverChange={(hovered) => setHoverPresence("trigger", hovered)}
      usesDrawer={usesDrawer}
    />
  );

  if (usesDrawer) {
    return (
      <AgentGoalDrawer detailsId={detailsId} goal={goal} open={open} setOpen={setOpen}>
        {trigger}
      </AgentGoalDrawer>
    );
  }

  return (
    <AgentGoalPopover
      detailsId={detailsId}
      goal={goal}
      open={open}
      setOpen={setOpen}
      suppressHoverRef={suppressHoverRef}
      triggerRef={triggerRef}
      triggerWasFocusedRef={triggerWasFocusedRef}
      restoringFocusRef={restoringFocusRef}
      onHoverChange={(hovered) => setHoverPresence("content", hovered)}
    >
      {trigger}
    </AgentGoalPopover>
  );
}
