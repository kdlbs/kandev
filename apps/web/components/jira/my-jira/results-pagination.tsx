"use client";

import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@kandev/ui/pagination";

type ResultsPaginationProps = {
  page: number;
  pageSize: number;
  itemCount: number;
  isLast: boolean;
  onNext: () => void;
  onPrev: () => void;
};

// Token-based pagination from Atlassian's /search/jql gives no total count, so
// we render forward/backward controls plus the current page's row range.
export function ResultsPagination({
  page,
  pageSize,
  itemCount,
  isLast,
  onNext,
  onPrev,
}: ResultsPaginationProps) {
  if (page === 1 && isLast) return null;
  const start = (page - 1) * pageSize + 1;
  const end = (page - 1) * pageSize + itemCount;
  const prevDisabled = page <= 1;
  const nextDisabled = isLast;

  return (
    <div className="flex items-center justify-between px-6 py-3 border-t shrink-0">
      <div className="text-xs text-muted-foreground tabular-nums">
        {itemCount === 0 ? "No results" : `${start}–${end}`}
      </div>
      <Pagination className="mx-0 w-auto justify-end">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!prevDisabled) onPrev();
              }}
              aria-disabled={prevDisabled}
              className={prevDisabled ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
          <PaginationItem>
            <span className="px-3 text-sm tabular-nums">Page {page}</span>
          </PaginationItem>
          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!nextDisabled) onNext();
              }}
              aria-disabled={nextDisabled}
              className={nextDisabled ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
