"use client";

import { IconLoader2, IconSearch, IconX } from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { PanelHeaderBar } from "./panel-primitives";

type FileBrowserSearchHeaderProps = {
  isSearching: boolean;
  localSearchQuery: string;
  searchInputRef: React.RefObject<HTMLInputElement | null>;
  onSearchChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onCloseSearch: () => void;
};

export function FileBrowserSearchHeader({
  isSearching,
  localSearchQuery,
  searchInputRef,
  onSearchChange,
  onCloseSearch,
}: FileBrowserSearchHeaderProps) {
  return (
    <PanelHeaderBar className="group/header">
      {isSearching ? (
        <IconLoader2 className="h-3.5 w-3.5 text-muted-foreground animate-spin shrink-0" />
      ) : (
        <IconSearch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      )}
      <Input
        ref={searchInputRef}
        type="text"
        value={localSearchQuery}
        onChange={onSearchChange}
        onKeyDown={(e) => {
          if (e.key === "Escape") onCloseSearch();
        }}
        placeholder="Search files..."
        className="flex-1 min-w-0 h-5 text-xs border-none bg-transparent shadow-none focus-visible:ring-0 px-2"
      />
      <button
        className="text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
        onClick={onCloseSearch}
      >
        <IconX className="h-3.5 w-3.5" />
      </button>
    </PanelHeaderBar>
  );
}
