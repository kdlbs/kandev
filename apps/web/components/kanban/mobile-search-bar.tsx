"use client";

import { TaskSearchInput } from "./task-search-input";

type MobileSearchBarProps = {
  searchQuery: string;
  onSearchChange: (query: string) => void;
};

export function MobileSearchBar({ searchQuery, onSearchChange }: MobileSearchBarProps) {
  return (
    <div className="border-b border-border px-4 py-2" data-testid="mobile-search-bar">
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder="Search tasks..."
        className="w-full"
        autoFocus
      />
    </div>
  );
}
