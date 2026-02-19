import { useCallback, useState } from "react";

const STORAGE_KEY = "kanban-swimlane-collapse";

function loadCollapseState(): Record<string, boolean> {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function saveCollapseState(state: Record<string, boolean>) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    // Ignore storage errors
  }
}

export function useSwimlaneCollapse() {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapseState);

  const isCollapsed = useCallback(
    (workflowId: string) => collapsed[workflowId] ?? false,
    [collapsed],
  );

  const toggleCollapse = useCallback((workflowId: string) => {
    setCollapsed((prev) => {
      const next = { ...prev, [workflowId]: !prev[workflowId] };
      saveCollapseState(next);
      return next;
    });
  }, []);

  return { isCollapsed, toggleCollapse };
}
