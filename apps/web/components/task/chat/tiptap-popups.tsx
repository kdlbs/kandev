"use client";

import { useMemo } from "react";
import { EntityReferenceMenu } from "./entity-reference-menu";
import { MentionMenu } from "./mention-menu";
import { MessageHistorySearch } from "./message-history-search";
import type { MessageHistoryEntry } from "./message-history";
import { SlashCommandMenu } from "./slash-command-menu";
import type { SlashCommand } from "./slash-command-types";
import type { useEntityReferenceComposer } from "./use-entity-reference-composer";
import type { MenuHandlers, ReverseSearchOverlay } from "./tiptap-input";

type TipTapPopupsProps = {
  menu: MenuHandlers;
  slashCommands: SlashCommand[];
  entityReferences: ReturnType<typeof useEntityReferenceComposer>;
  overlay: ReverseSearchOverlay;
  history: readonly MessageHistoryEntry[];
  isDraining: boolean;
  onReverseSearchSelect: (index: number) => void;
  onEntityReferenceClose: () => void;
};

export function TipTapPopups({
  menu,
  slashCommands,
  entityReferences,
  overlay,
  history,
  isDraining,
  onReverseSearchSelect,
  onEntityReferenceClose,
}: TipTapPopupsProps) {
  const currentSlashCommands = useMemo(() => {
    const byId = new Map(slashCommands.map((command) => [command.id, command]));
    return menu.slashMenu.items.map((item) => byId.get(item.id) ?? item);
  }, [slashCommands, menu.slashMenu.items]);

  return (
    <>
      <MentionMenu
        isOpen={menu.mentionMenu.isOpen}
        isLoading={false}
        clientRect={menu.mentionMenu.clientRect}
        items={menu.mentionMenu.items}
        query={menu.mentionMenu.query}
        selectedIndex={menu.mentionSelectedIndex}
        onSelect={menu.handleMentionSelect}
        onClose={menu.handleMentionClose}
        setSelectedIndex={menu.setMentionSelectedIndex}
      />
      <EntityReferenceMenu
        isOpen={entityReferences.isOpen}
        clientRect={entityReferences.clientRect}
        groups={entityReferences.groups}
        query={entityReferences.query}
        selectedIndex={entityReferences.selectedIndex}
        isSearching={entityReferences.isSearching}
        error={entityReferences.error}
        onRetry={entityReferences.retry}
        onSelect={entityReferences.selectReference}
        onClose={onEntityReferenceClose}
        setSelectedIndex={entityReferences.setSelectedIndex}
      />
      <SlashCommandMenu
        isOpen={menu.slashMenu.isOpen}
        clientRect={menu.slashMenu.clientRect}
        commands={currentSlashCommands}
        selectedIndex={menu.slashSelectedIndex}
        onSelect={menu.handleSlashSelect}
        onClose={menu.handleSlashClose}
        setSelectedIndex={menu.setSlashSelectedIndex}
      />
      {overlay.isReverseSearchOpen && overlay.reverseSearchContainer && (
        <MessageHistorySearch
          history={history}
          isLoadingOlder={isDraining}
          anchorRect={overlay.reverseSearchAnchor}
          container={overlay.reverseSearchContainer}
          onClose={overlay.closeReverseSearch}
          onEscapeDismiss={overlay.closeReverseSearchAndFocusEditor}
          onSelect={onReverseSearchSelect}
        />
      )}
    </>
  );
}
