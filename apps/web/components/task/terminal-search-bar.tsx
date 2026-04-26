"use client";

import { PanelSearchBar } from "@/components/search/panel-search-bar";
import type { TerminalSearchState } from "./use-terminal-search";

export function TerminalSearchBar({ search }: { search: TerminalSearchState }) {
  if (!search.isOpen) return null;
  return (
    <PanelSearchBar
      value={search.query}
      onChange={search.setQuery}
      onNext={search.findNext}
      onPrev={search.findPrev}
      onClose={search.close}
      matchInfo={search.matchInfo}
      hasError={search.hasError}
      errorText={search.errorText}
      toggles={{
        caseSensitive: { value: search.caseSensitive, onChange: search.setCaseSensitive },
        regex: { value: search.regex, onChange: search.setRegex },
      }}
    />
  );
}
