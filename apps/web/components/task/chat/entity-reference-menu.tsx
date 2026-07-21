"use client";

import { IconBug, IconClipboardList, IconGitPullRequest, IconLink } from "@tabler/icons-react";
import type { EntityReferenceSearchError } from "@/hooks/use-entity-reference-search";
import type {
  EntityReference,
  EntityReferenceGroupStatus,
  EntityReferenceSearchGroup,
} from "@/lib/types/entity-reference";
import { PopupMenu, PopupMenuItem, useMenuItemRefs } from "./popup-menu";

type EntityReferenceMenuProps = {
  isOpen: boolean;
  clientRect?: (() => DOMRect | null) | null;
  groups: EntityReferenceSearchGroup[];
  query: string;
  selectedIndex: number;
  isSearching: boolean;
  error: EntityReferenceSearchError | null;
  onRetry: () => void;
  onSelect: (reference: EntityReference) => void;
  onClose: () => void;
  setSelectedIndex: (index: number) => void;
};

const GROUP_STATUS_LABELS: Partial<Record<EntityReferenceGroupStatus, string>> = {
  unauthorized: "Sign-in required",
  rate_limited: "Temporarily rate limited",
  timeout: "Search timed out",
  upstream_error: "Provider unavailable",
  unsupported_scope: "Unavailable for this workspace",
};

function entityReferenceIcon(kind: string) {
  if (kind === "task") return <IconClipboardList className="h-4 w-4" />;
  if (kind === "issue" || kind === "work_item") return <IconBug className="h-4 w-4" />;
  if (kind === "pull_request" || kind === "merge_request") {
    return <IconGitPullRequest className="h-4 w-4" />;
  }
  return <IconLink className="h-4 w-4" data-testid="entity-reference-generic-icon" />;
}

function selectableReferences(groups: readonly EntityReferenceSearchGroup[]): EntityReference[] {
  return groups.flatMap((group) => (group.status === "ok" ? group.results : []));
}

function entityReferenceEmptyState(
  query: string,
  isSearching: boolean,
  error: EntityReferenceSearchError | null,
  onRetry: () => void,
) {
  if (!query.trim()) return "Type to search work items";
  if (isSearching) return "Searching work items…";
  if (error) {
    return (
      <div className="flex min-h-11 items-center justify-between gap-3 px-3 text-xs">
        <span className="text-muted-foreground">{error.message}</span>
        <button
          type="button"
          className="min-h-11 shrink-0 cursor-pointer px-2 font-medium text-primary"
          onClick={onRetry}
        >
          Retry
        </button>
      </div>
    );
  }
  return "No work items found";
}

export function EntityReferenceMenu({
  isOpen,
  clientRect,
  groups,
  query,
  selectedIndex,
  isSearching,
  error,
  onRetry,
  onSelect,
  onClose,
  setSelectedIndex,
}: EntityReferenceMenuProps) {
  const { setItemRef } = useMenuItemRefs(selectedIndex);
  const selectable = selectableReferences(groups);
  let itemIndex = 0;
  const visibleGroups = groups.filter(
    (group) =>
      group.status !== "not_configured" && (group.status !== "ok" || group.results.length > 0),
  );
  const hasContent = visibleGroups.length > 0;
  return (
    <PopupMenu
      isOpen={isOpen}
      testId="entity-reference-menu"
      position={null}
      clientRect={clientRect}
      title="Reference work items"
      selectedIndex={selectedIndex}
      onClose={onClose}
      hasItems={hasContent}
      emptyState={
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {entityReferenceEmptyState(query, isSearching, error, onRetry)}
        </div>
      }
    >
      {visibleGroups.map((group) => (
        <div key={group.source} data-reference-group={group.source}>
          <div className="flex items-center justify-between gap-2 px-3 py-1 text-[11px] text-muted-foreground">
            <span className="truncate font-medium">{group.display_name}</span>
            <span className="shrink-0">{group.kind_label}</span>
          </div>
          {group.status === "ok" ? (
            group.results.map((reference) => {
              const index = itemIndex++;
              return (
                <PopupMenuItem
                  key={reference.ref}
                  icon={entityReferenceIcon(group.kind)}
                  label={`#${reference.key || reference.title}`}
                  description={reference.title}
                  isSelected={selectedIndex === index}
                  onClick={() => onSelect(reference)}
                  onMouseEnter={() => setSelectedIndex(index)}
                  itemRef={setItemRef(index)}
                />
              );
            })
          ) : (
            <div className="min-h-11 px-3 py-2 text-xs text-muted-foreground">
              {GROUP_STATUS_LABELS[group.status] ?? "Provider unavailable"}
            </div>
          )}
        </div>
      ))}
      {isSearching && selectable.length > 0 && (
        <div className="px-3 py-1 text-[11px] text-muted-foreground">Updating…</div>
      )}
    </PopupMenu>
  );
}
