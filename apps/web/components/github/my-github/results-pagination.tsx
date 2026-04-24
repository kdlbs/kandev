"use client";

import { Fragment } from "react";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@kandev/ui/pagination";

// GitHub's search API caps total_count at 1000 regardless of actual results.
const GITHUB_MAX_RESULTS = 1000;

type ResultsPaginationProps = {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
};

function pageWindow(page: number, totalPages: number): number[] {
  const pages = new Set<number>([1, totalPages, page - 1, page, page + 1]);
  return Array.from(pages)
    .filter((p) => p >= 1 && p <= totalPages)
    .sort((a, b) => a - b);
}

export function ResultsPagination({ page, pageSize, total, onPageChange }: ResultsPaginationProps) {
  const effectiveTotal = Math.min(total, GITHUB_MAX_RESULTS);
  const totalPages = Math.max(1, Math.ceil(effectiveTotal / pageSize));
  if (totalPages <= 1) return null;

  const windowPages = pageWindow(page, totalPages);
  const start = (page - 1) * pageSize + 1;
  const end = Math.min(page * pageSize, effectiveTotal);

  return (
    <div className="flex items-center justify-between px-6 py-3 border-t shrink-0">
      <div className="text-xs text-muted-foreground tabular-nums">
        {start}–{end} of {total > GITHUB_MAX_RESULTS ? `${GITHUB_MAX_RESULTS}+` : total}
      </div>
      <Pagination className="mx-0 w-auto justify-end">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (page > 1) onPageChange(page - 1);
              }}
              aria-disabled={page <= 1}
              className={page <= 1 ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
          {windowPages.map((p, i) => {
            const prev = windowPages[i - 1];
            const needsGap = prev !== undefined && p - prev > 1;
            return (
              <Fragment key={p}>
                {needsGap && (
                  <PaginationItem>
                    <PaginationEllipsis />
                  </PaginationItem>
                )}
                <PaginationItem>
                  <PaginationLink
                    href="#"
                    isActive={p === page}
                    onClick={(e) => {
                      e.preventDefault();
                      onPageChange(p);
                    }}
                    className="cursor-pointer"
                  >
                    {p}
                  </PaginationLink>
                </PaginationItem>
              </Fragment>
            );
          })}
          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (page < totalPages) onPageChange(page + 1);
              }}
              aria-disabled={page >= totalPages}
              className={page >= totalPages ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
