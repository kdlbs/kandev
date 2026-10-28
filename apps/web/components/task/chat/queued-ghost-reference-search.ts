import { useCallback, useMemo, useRef, useState } from "react";
import { useEntityReferenceSearch } from "@/hooks/use-entity-reference-search";
import { useTask } from "@/hooks/use-task";
import type { EntityReference } from "@/lib/types/entity-reference";
import { selectQueueReference } from "@/hooks/domains/session/queue-registration";
import { selectableEntityReferences } from "./entity-reference-groups";
import { createEntityReferenceSelection } from "./reference-registration";

type QueuedReferenceSearchArgs = {
  taskId: string;
  sessionId: string;
  value: string;
  references: readonly EntityReference[];
  onValueChange: (value: string) => void;
  onReferencesChange: (references: EntityReference[]) => void;
};

export function useQueuedGhostReferenceSearch({
  taskId,
  sessionId,
  value,
  references,
  onValueChange,
  onReferencesChange,
}: QueuedReferenceSearchArgs) {
  const task = useTask(taskId, false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const search = useEntityReferenceSearch({
    workspaceId: task?.workspaceId,
    sessionId,
    query,
    enabled: searchOpen,
  });
  const selectable = useMemo(() => selectableEntityReferences(search.groups), [search.groups]);
  const boundedIndex = Math.min(selectedIndex, Math.max(0, selectable.length - 1));
  const clientRect = useCallback(() => inputRef.current?.getBoundingClientRect() ?? null, []);
  const selectReference = useCallback(
    (reference: EntityReference) => {
      const selection = createEntityReferenceSelection(reference);
      const next = selectQueueReference(value, references, selection);
      onValueChange(next.value);
      onReferencesChange(next.references);
      setQuery("");
      setSearchOpen(false);
      setSelectedIndex(0);
    },
    [onReferencesChange, onValueChange, references, value],
  );
  const handleSearchKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setSearchOpen(false);
        return;
      }
      if (event.key === "ArrowDown" && selectable.length > 0) {
        event.preventDefault();
        setSelectedIndex((index) => Math.min(index + 1, selectable.length - 1));
      } else if (event.key === "ArrowUp" && selectable.length > 0) {
        event.preventDefault();
        setSelectedIndex((index) => Math.max(index - 1, 0));
      } else if (event.key === "Enter" && selectable[boundedIndex]) {
        event.preventDefault();
        selectReference(selectable[boundedIndex]);
      }
    },
    [boundedIndex, selectable, selectReference],
  );

  return {
    inputRef,
    query,
    setQuery,
    searchOpen,
    setSearchOpen,
    selectedIndex: boundedIndex,
    setSelectedIndex,
    selectable,
    search,
    clientRect,
    selectReference,
    handleSearchKeyDown,
  };
}
