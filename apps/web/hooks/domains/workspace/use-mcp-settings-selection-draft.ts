"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMCPSelectionEditor } from "./use-mcp-selection-editor";

function sameIds(left: string[], right: string[]) {
  return left.length === right.length && left.every((id) => right.includes(id));
}
type Draft = { key: string; ids: string[]; baseline: string[] };

/** Settings owns persistence; an acknowledged save cannot discard newer edits. */
export function useMCPSettingsSelectionDraft(
  scope: "profile" | "repository",
  ownerId: string,
  workspaceId: string | null,
) {
  const editor = useMCPSelectionEditor(scope, ownerId, workspaceId);
  const key = JSON.stringify([scope, ownerId, workspaceId]);
  const [draft, setDraft] = useState<Draft>({ key, ids: [], baseline: [] });
  const current = draft.key === key ? draft : { key, ids: [], baseline: [] };
  const context = useRef(key);
  const draftRef = useRef(current);
  useLayoutEffect(() => {
    context.current = key;
    draftRef.current = current;
  });
  useEffect(() => {
    const saved = editor.selection;
    if (!saved) return;
    setDraft((previous) => ({
      key,
      baseline: saved.definition_ids,
      ids:
        previous.key !== key || sameIds(previous.ids, previous.baseline)
          ? saved.definition_ids
          : previous.ids,
    }));
  }, [key, editor.selection]);
  const setSelectedIds = useCallback(
    (ids: string[]) => {
      setDraft((previous) => ({
        ...(previous.key === key ? previous : { key, baseline: [] }),
        ids,
      }));
    },
    [key],
  );
  const discard = useCallback(() => {
    setDraft((previous) =>
      previous.key === key ? { ...previous, ids: previous.baseline } : previous,
    );
  }, [key]);
  const save = async () => {
    const submitted = draftRef.current.ids;
    const saved = await editor.save(submitted);
    if (!saved || context.current !== key) return null;
    setDraft((previous) =>
      previous.key !== key
        ? previous
        : {
            key,
            baseline: saved.definition_ids,
            ids: sameIds(previous.ids, submitted) ? saved.definition_ids : previous.ids,
          },
    );
    return saved;
  };
  return {
    editor,
    selectedIds: current.ids,
    dirty: !sameIds(current.ids, current.baseline),
    setSelectedIds,
    discard,
    save,
  };
}
