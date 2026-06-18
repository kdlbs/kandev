type ShouldSkipInitialTasksFetchParams = {
  hasInitialData: boolean;
  alreadySkipped: boolean;
  pageIndex: number;
  debouncedQuery: string;
  showArchived: boolean;
};

export function shouldSkipInitialTasksFetch({
  hasInitialData,
  alreadySkipped,
  pageIndex,
  debouncedQuery,
  showArchived,
}: ShouldSkipInitialTasksFetchParams): boolean {
  return (
    hasInitialData && !alreadySkipped && pageIndex === 0 && debouncedQuery === "" && !showArchived
  );
}
