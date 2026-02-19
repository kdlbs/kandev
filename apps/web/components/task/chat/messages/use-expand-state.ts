import { useState, useCallback } from "react";

/**
 * Manages expandable row state for tool message components.
 * Handles manual override, auto-expand behavior, and status-based reset.
 *
 * Uses React's "store previous value in state" pattern to reset manual state
 * when status changes, avoiding ref mutation during render.
 *
 * @param status - Current tool status (used to reset manual state on status change)
 * @param autoExpanded - Whether the row should be auto-expanded (e.g. when running)
 */
export function useExpandState(status: string | undefined, autoExpanded: boolean) {
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const [prevStatus, setPrevStatus] = useState(status);

  // Reset manual state when status changes (allows auto-expand behavior to resume)
  // This uses React's recommended pattern for deriving state from props changes.
  if (prevStatus !== status) {
    setPrevStatus(status);
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  return { isExpanded, handleToggle };
}
