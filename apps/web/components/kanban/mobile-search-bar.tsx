"use client";

import { TaskSearchInput } from "./task-search-input";
import { useTranslation } from "react-i18next";

type MobileSearchBarProps = {
  searchQuery: string;
  onSearchChange: (query: string) => void;
};

export function MobileSearchBar({ searchQuery, onSearchChange }: MobileSearchBarProps) {
  const { t } = useTranslation();
  return (
    <div className="border-b border-border px-4 py-2" data-testid="mobile-search-bar">
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder={t("kanban:searchTasksPlaceholder")}
        className="w-full"
        autoFocus
      />
    </div>
  );
}
