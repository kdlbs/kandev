import { useRef, useState, type KeyboardEvent } from "react";
import { formatShortcut, matchesShortcut } from "@/lib/keyboard/utils";

const MOVE_SHORTCUT = { key: "Enter", modifiers: { ctrlOrCmd: true } } as const;

export function workflowMoveShortcutLabel() {
  return formatShortcut(MOVE_SHORTCUT);
}

/** A local submission boundary shared by the Move button and its options fields. */
export function useWorkflowMoveSubmit(isMoving: boolean, onSubmit: () => Promise<unknown>) {
  const inFlight = useRef(false);
  const [submitting, setSubmitting] = useState(false);
  const busy = isMoving || submitting;
  const submit = async () => {
    if (isMoving || inFlight.current) return;
    inFlight.current = true;
    setSubmitting(true);
    try {
      await onSubmit();
    } finally {
      inFlight.current = false;
      setSubmitting(false);
    }
  };
  const onKeyDown = (event: KeyboardEvent) => {
    if (
      event.defaultPrevented ||
      event.repeat ||
      event.nativeEvent.isComposing ||
      event.keyCode === 229 ||
      !matchesShortcut(event, MOVE_SHORTCUT)
    )
      return;
    event.preventDefault();
    event.stopPropagation();
    void submit();
  };
  return { busy, submit, onKeyDown };
}
