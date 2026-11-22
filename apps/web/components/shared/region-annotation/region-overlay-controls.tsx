"use client";

import { useCallback, useRef, useState } from "react";
import { IconAlertTriangle, IconEye, IconEyeOff } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { cn } from "@/lib/utils";

/** Host-translated copy for the drift warning and the saved-capture toggle. */
export type RegionOverlayLabels = {
  drifted: string;
  showCapture: string;
  hideCapture: string;
};

/** A press shorter than this toggles the capture; a longer one only shows it while held. */
const HOLD_MS = 250;

const CONTROL_CLASS =
  "pointer-events-auto flex size-6 cursor-pointer items-center justify-center rounded-full border bg-background shadow-sm [@media(pointer:coarse)]:size-11";

/** Whether a region shows its saved capture: while its button is held, or until pressed again. */
export function useCaptureView() {
  const [pinned, setPinned] = useState(false);
  const [holding, setHolding] = useState(false);
  const pressedAt = useRef(0);
  return {
    visible: pinned || holding,
    hide: useCallback(() => {
      setPinned(false);
      setHolding(false);
    }, []),
    buttonProps: {
      onPointerDown: (event: React.PointerEvent) => {
        event.stopPropagation();
        pressedAt.current = Date.now();
        setHolding(true);
      },
      onPointerUp: () => setHolding(false),
      onPointerLeave: () => setHolding(false),
      onClick: (event: React.MouseEvent) => {
        event.stopPropagation();
        const held = pressedAt.current && Date.now() - pressedAt.current >= HOLD_MS;
        pressedAt.current = 0;
        if (!held) setPinned((current) => !current);
      },
    },
  };
}

export type CaptureView = ReturnType<typeof useCaptureView>;

export function CaptureToggle({
  view,
  labels,
}: {
  view: CaptureView;
  labels: RegionOverlayLabels;
}) {
  const Icon = view.visible ? IconEyeOff : IconEye;
  return (
    <button
      type="button"
      aria-label={view.visible ? labels.hideCapture : labels.showCapture}
      aria-pressed={view.visible}
      data-testid="region-capture-toggle"
      className={cn(
        CONTROL_CLASS,
        "opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100",
        view.visible && "opacity-100",
      )}
      onMouseDown={(event) => event.stopPropagation()}
      {...view.buttonProps}
    >
      <Icon className="pointer-events-none size-3.5" aria-hidden="true" />
    </button>
  );
}

export function CaptureImage({ url }: { url: string }) {
  return (
    <img
      src={url}
      alt=""
      data-testid="region-capture-image"
      className="pointer-events-none absolute inset-0 size-full bg-background object-contain"
    />
  );
}

/** A warning badge whose tooltip, or tap-opened popover on touch, explains the drift. */
export function DriftWarning({ label }: { label: string }) {
  const touch = useTouchDrawer();
  const button = (
    <button
      type="button"
      aria-label={label}
      data-testid="region-drift-warning"
      className={cn(CONTROL_CLASS, "border-amber-500 text-amber-600")}
      onPointerDown={(event) => event.stopPropagation()}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
    >
      <IconAlertTriangle className="pointer-events-none size-3.5" aria-hidden="true" />
    </button>
  );
  const content = <p className="max-w-72 text-xs leading-relaxed">{label}</p>;
  if (touch) {
    return (
      <Popover>
        <PopoverTrigger asChild>{button}</PopoverTrigger>
        <PopoverContent className="w-auto max-w-80 p-3">{content}</PopoverContent>
      </Popover>
    );
  }
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>{content}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
