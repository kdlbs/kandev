"use client";

import { IconLoader2 } from "@tabler/icons-react";
import { CommandEmpty, CommandGroup, CommandItem } from "@kandev/ui/command";
import { FileIcon } from "@/components/ui/file-icon";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import type { WorkspaceContentSearchError } from "@/hooks/domains/session/use-workspace-content-search";
import { groupByRepositoryName, isSingleRepoGroup } from "@/lib/group-by-repo";
import type { ContentSearchMatchRange, WorkspaceContentSearchResult } from "@/lib/types/backend";
import { useTranslation } from "react-i18next";

type PreviewPart = { text: string; matched: boolean };

function normalizeMatchRanges(
  previewLength: number,
  ranges: ContentSearchMatchRange[],
): ContentSearchMatchRange[] {
  const normalized = ranges
    .filter(({ start, end }) => Number.isFinite(start) && Number.isFinite(end) && end > start)
    .map(({ start, end }) => ({
      start: Math.max(0, Math.min(previewLength, Math.floor(start))),
      end: Math.max(0, Math.min(previewLength, Math.floor(end))),
    }))
    .filter(({ start, end }) => end > start)
    .sort((a, b) => a.start - b.start || a.end - b.end);

  const merged: ContentSearchMatchRange[] = [];
  for (const range of normalized) {
    const previous = merged.at(-1);
    if (previous && range.start <= previous.end) {
      previous.end = Math.max(previous.end, range.end);
    } else {
      merged.push({ ...range });
    }
  }
  return merged;
}

export function splitPreviewByMatches(
  preview: string,
  ranges: ContentSearchMatchRange[],
): PreviewPart[] {
  const parts: PreviewPart[] = [];
  let cursor = 0;
  for (const range of normalizeMatchRanges(preview.length, ranges)) {
    if (range.start > cursor) {
      parts.push({ text: preview.slice(cursor, range.start), matched: false });
    }
    parts.push({ text: preview.slice(range.start, range.end), matched: true });
    cursor = range.end;
  }
  if (cursor < preview.length) parts.push({ text: preview.slice(cursor), matched: false });
  return parts;
}

export function getContentSearchResultValue(result: WorkspaceContentSearchResult): string {
  return `__content:${JSON.stringify([
    result.repository_name,
    result.path,
    result.line,
    result.column,
  ])}`;
}

function getFileName(path: string): string {
  return path.split("/").pop() || path;
}

function HighlightedPreview({ result }: { result: WorkspaceContentSearchResult }) {
  return (
    <span
      data-testid="content-search-preview"
      className="block truncate font-mono text-[0.6875rem] leading-4 text-muted-foreground"
    >
      {splitPreviewByMatches(result.preview, result.match_ranges).map((part, index) =>
        part.matched ? (
          <mark
            key={`${index}:${part.text}`}
            data-testid="content-search-match"
            className="rounded-[2px] bg-primary/15 px-0 text-foreground"
          >
            {part.text}
          </mark>
        ) : (
          part.text
        ),
      )}
    </span>
  );
}

function SearchResultRow({
  result,
  onSelect,
}: {
  result: WorkspaceContentSearchResult;
  onSelect: (result: WorkspaceContentSearchResult) => void;
}) {
  return (
    <CommandItem
      value={getContentSearchResultValue(result)}
      onSelect={() => onSelect(result)}
      forceMount
      data-testid="content-search-result"
      data-path={result.path}
      data-repository={result.repository_name}
      data-line={result.line}
      className="min-h-10 cursor-pointer items-start py-2"
    >
      <FileIcon fileName={getFileName(result.path)} className="mt-0.5 shrink-0" />
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="truncate font-medium">{result.path}</span>
          <span className="ml-auto shrink-0 font-mono text-[0.625rem] text-muted-foreground tabular-nums">
            {result.line}
          </span>
        </span>
        <HighlightedPreview result={result} />
      </span>
    </CommandItem>
  );
}

type WorkspaceContentSearchProps = {
  results: WorkspaceContentSearchResult[];
  isSearching: boolean;
  error: WorkspaceContentSearchError | null;
  search: string;
  sessionId: string | null;
  onSelect: (result: WorkspaceContentSearchResult) => void;
};

export function WorkspaceContentSearch({
  results,
  isSearching,
  error,
  search,
  sessionId,
  onSelect,
}: WorkspaceContentSearchProps) {
  const { t } = useTranslation();
  const getRepoDisplayName = useRepoDisplayName(sessionId);
  if (error === "session-unavailable") {
    return <CommandEmpty>{t("common:contentSearchNeedsAnActiveTask")}</CommandEmpty>;
  }
  if (error === "query-too-long") {
    return <CommandEmpty>{t("common:searchQueriesAreLimitedTo200")}</CommandEmpty>;
  }
  if (error === "transport-error") {
    return <CommandEmpty>{t("common:searchFailedEditTheQueryOr")}</CommandEmpty>;
  }
  if (isSearching && results.length === 0) {
    return (
      <CommandEmpty>
        <IconLoader2 className="mr-2 inline size-3.5 animate-spin text-muted-foreground" />
        {t("common:searchingTaskWorkspace")}
      </CommandEmpty>
    );
  }
  if (!search.trim()) return <CommandEmpty>{t("common:typeToSearchTaskContents")}</CommandEmpty>;
  if (results.length === 0) return <CommandEmpty>{t("common:noContentMatchesFound")}</CommandEmpty>;

  const groups = groupByRepositoryName(results, (result) => result.repository_name);
  const singleRepo = isSingleRepoGroup(groups);
  // Results are published as each retry attempt returns, so a populated list
  // can still be growing. The spec requires the searching state to stay
  // distinguishable from a finished one, and without this a partial list looks
  // identical to "that is everything" while another repository is still
  // starting up.
  //
  // It leads the list and sticks to the top of CommandList's own scroll box
  // (max-h-72 overflow-y-auto): appended after the groups it fell below the
  // fold as soon as the early matches filled the palette, which is exactly the
  // case it exists for.
  const stillSearching = isSearching ? (
    <div
      key="content-search-in-progress"
      data-testid="content-search-in-progress"
      className="sticky top-0 z-10 flex items-center bg-popover px-2 py-1.5 text-xs text-muted-foreground"
    >
      <IconLoader2 className="mr-2 inline size-3 animate-spin" />
      {t("common:searchingTaskWorkspace")}
    </div>
  ) : null;
  return [
    stillSearching,
    ...groups.map((group) => (
      <CommandGroup
        key={group.repositoryName}
        heading={
          singleRepo
            ? t("common:results")
            : (getRepoDisplayName(group.repositoryName) ?? t("common:workspace"))
        }
        forceMount
        data-testid="content-search-repo-group"
        data-repository={group.repositoryName}
      >
        {group.items.map((result) => (
          <SearchResultRow
            key={getContentSearchResultValue(result)}
            result={result}
            onSelect={onSelect}
          />
        ))}
      </CommandGroup>
    )),
  ];
}
