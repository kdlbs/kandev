import { useRef, useState, type ComponentProps } from "react";
import type { Button } from "@kandev/ui/button";

export function useSessionMenuOpeningGesture() {
  const [open, setOpen] = useState(false);
  const openingTouch = useRef(false);
  const triggerProps: Pick<
    ComponentProps<typeof Button>,
    "onPointerDown" | "onPointerCancel" | "onClick" | "onKeyDown"
  > = {
    onPointerDown: (event) => {
      openingTouch.current = event.pointerType === "touch";
      // Keep items absent until the opening touch has completed on the trigger.
      if (openingTouch.current) event.preventDefault();
    },
    onPointerCancel: () => {
      openingTouch.current = false;
    },
    onClick: (event) => {
      event.stopPropagation();
      if (!openingTouch.current) return;
      openingTouch.current = false;
      setOpen((wasOpen) => !wasOpen);
    },
    onKeyDown: (event) => {
      openingTouch.current = false;
      event.stopPropagation();
    },
  };
  return { open, setOpen, triggerProps };
}
