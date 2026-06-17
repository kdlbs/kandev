"use client";

import { IconFile, IconRobot } from "@tabler/icons-react";
import {
  suggestionDescription,
  suggestionLabel,
  type SuggestionItem,
} from "./passthrough-composer-state";
import type { PassthroughSuggestionState } from "./passthrough-composer-helpers";

export function PassthroughSuggestionMenu({
  open,
  suggestion,
  items,
  selectedIndex,
  setSelectedIndex,
  onSelect,
}: {
  open: boolean;
  suggestion: PassthroughSuggestionState;
  items: SuggestionItem[];
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
  onSelect: (item: SuggestionItem) => void;
}) {
  if (!open || !suggestion) return null;
  return (
    <div
      className="absolute bottom-14 left-2 z-20 w-80 max-w-[calc(100vw-1rem)] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md"
      data-testid="passthrough-composer-suggestions"
    >
      <div className="border-b px-3 py-1 text-xs font-medium text-muted-foreground">
        {suggestion.kind === "command" ? "Commands" : "Files"}
      </div>
      {items.map((item, index) => (
        <SuggestionButton
          key={suggestionLabel(item)}
          item={item}
          kind={suggestion.kind}
          selected={selectedIndex === index}
          onSelect={onSelect}
          onHover={() => setSelectedIndex(index)}
        />
      ))}
    </div>
  );
}

function SuggestionButton({
  item,
  kind,
  selected,
  onSelect,
  onHover,
}: {
  item: SuggestionItem;
  kind: "command" | "file";
  selected: boolean;
  onSelect: (item: SuggestionItem) => void;
  onHover: () => void;
}) {
  return (
    <button
      type="button"
      className={`flex w-full cursor-pointer items-start gap-2 px-3 py-2 text-left text-sm ${
        selected ? "bg-accent" : ""
      }`}
      onMouseEnter={onHover}
      onClick={() => onSelect(item)}
    >
      {kind === "command" ? (
        <IconRobot className="mt-0.5 h-4 w-4 shrink-0" />
      ) : (
        <IconFile className="mt-0.5 h-4 w-4 shrink-0" />
      )}
      <span className="min-w-0">
        <span className="block truncate">{suggestionLabel(item)}</span>
        {suggestionDescription(item) && (
          <span className="block truncate text-xs text-muted-foreground">
            {suggestionDescription(item)}
          </span>
        )}
      </span>
    </button>
  );
}
