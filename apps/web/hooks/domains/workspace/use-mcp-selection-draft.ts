import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { MCPSelectionResponse } from "@/lib/types/http-mcp";

function sameSelection(left: string[], right: string[]) {
  return left.length === right.length && left.every((id) => right.includes(id));
}

// A new accepted snapshot advances the baseline without replacing a local edit.
export function useMCPSelectionDraft(context: string, selection: MCPSelectionResponse | null) {
  const [draft, setDraft] = useState({ context, ids: [] as string[], baseline: [] as string[] });
  const contextRef = useRef(context);
  useLayoutEffect(() => {
    contextRef.current = context;
  }, [context]);
  useEffect(() => {
    setDraft((previous) => {
      const baseline = selection?.definition_ids ?? [];
      if (previous.context !== context) return { context, ids: baseline, baseline };
      if (!selection) return previous;
      const ids = sameSelection(previous.ids, previous.baseline) ? baseline : previous.ids;
      return { context, ids, baseline };
    });
  }, [context, selection]);
  const current = draft.context === context ? draft : { context, ids: [], baseline: [] };
  return {
    selectedIds: current.ids,
    dirty: Boolean(selection) && !sameSelection(current.ids, current.baseline),
    setSelectedIds: (ids: string[]) => setDraft((previous) => ({ ...previous, context, ids })),
    discard: () => setDraft((previous) => ({ ...previous, ids: previous.baseline })),
    acknowledge: (response: MCPSelectionResponse | null) => {
      if (!response || contextRef.current !== context) return;
      setDraft((previous) => ({ ...previous, baseline: response.definition_ids }));
    },
  };
}
