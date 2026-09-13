import { useCallback, useLayoutEffect, useRef, useState } from "react";
import type { WorkflowStep } from "@/components/kanban-column";

export function useCompactSwimlaneHeight(
  enabled: boolean,
  steps: WorkflowStep[],
  isDragging: boolean,
) {
  const [heights, setHeights] = useState<Record<string, number>>({});
  const scopeRef = useRef({ enabled, steps });
  scopeRef.current = { enabled, steps };
  const onNaturalHeightChange = useCallback((stepId: string, height: number) => {
    const scope = scopeRef.current;
    if (!scope.enabled || !scope.steps.some((step) => step.id === stepId)) return;
    setHeights((previous) =>
      previous[stepId] === height ? previous : { ...previous, [stepId]: height },
    );
  }, []);
  useLayoutEffect(() => {
    setHeights((previous) => {
      const entries = Object.entries(previous).filter(
        ([id]) => enabled && steps.some((step) => step.id === id),
      );
      return entries.length === Object.keys(previous).length
        ? previous
        : Object.fromEntries(entries);
    });
  }, [enabled, steps]);
  const naturalHeight = Math.max(0, ...steps.map((step) => heights[step.id] ?? 0));
  const nextHeight = `clamp(12.5rem, ${naturalHeight}px, 25rem)`;
  const settledHeight = useRef(nextHeight);
  useLayoutEffect(() => {
    if (!isDragging) settledHeight.current = nextHeight;
  }, [isDragging, nextHeight]);
  const columnHeight = isDragging ? settledHeight.current : nextHeight;
  return {
    columnHeight: enabled ? columnHeight : undefined,
    onNaturalHeightChange: enabled ? onNaturalHeightChange : undefined,
  };
}
