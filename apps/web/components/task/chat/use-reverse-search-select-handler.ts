import { useCallback } from "react";
import type { Editor } from "@tiptap/core";

export function useReverseSearchSelectHandler(
  applyHistoryEntry: (index: number) => void,
  closeReverseSearch: () => void,
  editor: Editor | null,
) {
  return useCallback(
    (index: number) => {
      applyHistoryEntry(index);
      closeReverseSearch();
      editor?.commands.focus("end");
    },
    [applyHistoryEntry, closeReverseSearch, editor],
  );
}
