"use client";

import type { Ref } from "react";
import { FileBrowserSearchHeader } from "./file-browser-search-header";
import { FileBrowserToolbar } from "./file-browser-parts";
import { useFileBrowserSearch } from "./file-browser-hooks";

export type FileBrowserHeaderProps = {
  treeLoaded: boolean;
  search: ReturnType<typeof useFileBrowserSearch>;
  displayPath: string;
  fullPath: string;
  copied: boolean;
  expandedPathsSize: number;
  onCopyPath: (value: string) => void | Promise<void>;
  onStartCreate?: () => void;
  onOpenFolder: () => void;
  onCollapseAll: () => void;
  showCreateButton: boolean;
  onUploadFiles?: (mode: "files" | "folder") => void;
  onAddSources?: (opener: HTMLButtonElement) => void;
  addSourcesButtonRef?: Ref<HTMLButtonElement>;
  addSourcesDisabledReason?: string;
};

export function FileBrowserHeader({
  treeLoaded,
  search,
  displayPath,
  fullPath,
  copied,
  expandedPathsSize,
  onCopyPath,
  onStartCreate,
  onOpenFolder,
  onCollapseAll,
  showCreateButton,
  onUploadFiles,
  onAddSources,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: FileBrowserHeaderProps) {
  if (!treeLoaded) return null;
  if (search.isSearchActive) {
    return (
      <FileBrowserSearchHeader
        isSearching={search.isSearching}
        localSearchQuery={search.localSearchQuery}
        searchInputRef={search.searchInputRef}
        onSearchChange={search.handleSearchChange}
        onCloseSearch={search.handleCloseSearch}
      />
    );
  }
  return (
    <FileBrowserToolbar
      displayPath={displayPath}
      fullPath={fullPath}
      copied={copied}
      expandedPathsSize={expandedPathsSize}
      onCopyPath={onCopyPath}
      onStartCreate={onStartCreate}
      onOpenFolder={onOpenFolder}
      onStartSearch={() => search.setIsSearchActive(true)}
      onCollapseAll={onCollapseAll}
      showCreateButton={showCreateButton}
      onUploadFiles={onUploadFiles}
      onAddSources={onAddSources}
      addSourcesButtonRef={addSourcesButtonRef}
      addSourcesDisabledReason={addSourcesDisabledReason}
    />
  );
}
