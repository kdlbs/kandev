import { useEffect, useRef, useState, type FocusEvent, type RefObject } from "react";

export type CompactWorkflowDisclosureControls = {
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
  setTriggerRef: (node: HTMLButtonElement | null) => void;
  contentRef: RefObject<HTMLDivElement | null>;
  openDisclosure: () => void;
  openDisclosureFromFocus: () => void;
  scheduleClose: () => void;
  cancelScheduledClose: () => void;
  handleTriggerFocus: () => void;
  handleTriggerBlur: (event: FocusEvent<HTMLButtonElement>) => void;
  handleContentFocus: () => void;
  handleContentBlur: (event: FocusEvent<HTMLDivElement>) => void;
  handleOpenAutoFocus: (event: Event) => void;
  handleCloseAutoFocus: (event: Event) => void;
};

function isElementWithin<T extends HTMLElement>(
  target: EventTarget | null,
  ref: RefObject<T | null>,
): boolean {
  return target instanceof Node && ref.current?.contains(target) === true;
}

function useCompactDisclosureCloseTimer(
  triggerRef: RefObject<HTMLButtonElement | null>,
  contentRef: RefObject<HTMLDivElement | null>,
  setOpen: (open: boolean) => void,
) {
  const closeTimerRef = useRef<number | null>(null);
  const cancelScheduledClose = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  };
  const scheduleClose = () => {
    cancelScheduledClose();
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      const activeElement = document.activeElement;
      const focusIsInsideDisclosure =
        isElementWithin(activeElement, triggerRef) || isElementWithin(activeElement, contentRef);
      if (!focusIsInsideDisclosure) setOpen(false);
    }, 100);
  };
  useEffect(
    () => () => {
      if (closeTimerRef.current !== null) window.clearTimeout(closeTimerRef.current);
    },
    [],
  );
  return { cancelScheduledClose, scheduleClose };
}

/**
 * Hover/focus open-close state for the compact workflow step disclosure. Keeps
 * the popover open while focus stays within the trigger or content, and returns
 * open the disclosure again on a click when it was already opened from focus.
 */
export function useCompactWorkflowDisclosure(): CompactWorkflowDisclosureControls {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const setTriggerRef = (node: HTMLButtonElement | null) => {
    triggerRef.current = node;
  };
  const contentRef = useRef<HTMLDivElement>(null);
  const suppressFocusOpenRef = useRef(false);
  const openedFromFocusRef = useRef(false);
  const contentHasFocusRef = useRef(false);
  const { cancelScheduledClose, scheduleClose } = useCompactDisclosureCloseTimer(
    triggerRef,
    contentRef,
    setOpen,
  );
  const openDisclosure = () => {
    openedFromFocusRef.current = false;
    cancelScheduledClose();
    setOpen(true);
  };
  const openDisclosureFromFocus = () => {
    openedFromFocusRef.current = !open;
    cancelScheduledClose();
    setOpen(true);
  };
  const handleTriggerFocus = () => {
    if (suppressFocusOpenRef.current) {
      suppressFocusOpenRef.current = false;
      return;
    }
    openDisclosureFromFocus();
  };
  const handleTriggerBlur = (event: FocusEvent<HTMLButtonElement>) => {
    suppressFocusOpenRef.current = false;
    if (isElementWithin(event.relatedTarget, contentRef)) {
      cancelScheduledClose();
      return;
    }
    scheduleClose();
  };
  const handleContentBlur = (event: FocusEvent<HTMLDivElement>) => {
    const relatedTarget = event.relatedTarget;
    if (isElementWithin(relatedTarget, contentRef)) {
      contentHasFocusRef.current = true;
      cancelScheduledClose();
      return;
    }
    contentHasFocusRef.current = false;
    if (isElementWithin(relatedTarget, triggerRef)) {
      cancelScheduledClose();
      return;
    }
    scheduleClose();
  };
  const handleContentFocus = () => {
    contentHasFocusRef.current = true;
    cancelScheduledClose();
  };
  const handleCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    if (contentHasFocusRef.current) {
      contentHasFocusRef.current = false;
      suppressFocusOpenRef.current = true;
      triggerRef.current?.focus();
    }
  };
  const handleOpenAutoFocus = (event: Event) => {
    if (!openedFromFocusRef.current) event.preventDefault();
    openedFromFocusRef.current = false;
  };
  return {
    open,
    setOpen,
    triggerRef,
    setTriggerRef,
    contentRef,
    openDisclosure,
    openDisclosureFromFocus,
    scheduleClose,
    cancelScheduledClose,
    handleTriggerFocus,
    handleTriggerBlur,
    handleContentFocus,
    handleContentBlur,
    handleOpenAutoFocus,
    handleCloseAutoFocus,
  };
}
