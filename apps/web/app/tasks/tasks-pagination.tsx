"use client";

import type { ReactNode } from "react";
import type { PaginationState } from "@tanstack/react-table";
import { Button } from "@kandev/ui/button";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
} from "@kandev/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  IconChevronLeft,
  IconChevronRight,
  IconChevronsLeft,
  IconChevronsRight,
} from "@tabler/icons-react";

type TasksPaginationProps = {
  total: number;
  pageCount: number;
  pagination: PaginationState;
  onPaginationChange: (
    next: PaginationState | ((prev: PaginationState) => PaginationState),
  ) => void;
};

type PageNumber = number | "ellipsis";

export function TasksPagination({
  total,
  pageCount,
  pagination,
  onPaginationChange,
}: TasksPaginationProps) {
  if (total === 0) return null;

  const currentPage = pagination.pageIndex + 1;
  const safePageCount = Math.max(1, pageCount);
  const pages = getPageNumbers(currentPage, safePageCount);
  const start = pagination.pageIndex * pagination.pageSize + 1;
  const end = Math.min((pagination.pageIndex + 1) * pagination.pageSize, total);
  const canPrevious = pagination.pageIndex > 0;
  const canNext = currentPage < safePageCount;
  const setPageIndex = (pageIndex: number) =>
    onPaginationChange((prev) => ({
      ...prev,
      pageIndex: Math.max(0, Math.min(pageIndex, safePageCount - 1)),
    }));

  return (
    <div
      data-testid="tasks-pagination"
      className="flex flex-col gap-3 px-1 pb-2 text-sm sm:flex-row sm:items-center sm:justify-between"
    >
      <div className="flex-1 text-muted-foreground">
        Showing {start} to {end} of {total} results
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-6 lg:gap-8">
        <RowsPerPageSelect pagination={pagination} onPaginationChange={onPaginationChange} />
        <Pagination className="mx-0 w-auto justify-start sm:justify-end">
          <PaginationContent>
            <PageNavigationButton
              label="Go to first page"
              disabled={!canPrevious}
              className="hidden lg:flex"
              onClick={() => setPageIndex(0)}
              icon={<IconChevronsLeft className="h-4 w-4" />}
            />
            <PageNavigationButton
              label="Go to previous page"
              disabled={!canPrevious}
              onClick={() => setPageIndex(pagination.pageIndex - 1)}
              icon={<IconChevronLeft className="h-4 w-4" />}
            />
            <PageButtons
              pages={pages}
              currentPage={currentPage}
              onPageChange={(page) => setPageIndex(page - 1)}
            />
            <PageNavigationButton
              label="Go to next page"
              disabled={!canNext}
              onClick={() => setPageIndex(pagination.pageIndex + 1)}
              icon={<IconChevronRight className="h-4 w-4" />}
            />
            <PageNavigationButton
              label="Go to last page"
              disabled={!canNext}
              className="hidden lg:flex"
              onClick={() => setPageIndex(safePageCount - 1)}
              icon={<IconChevronsRight className="h-4 w-4" />}
            />
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  );
}

function RowsPerPageSelect({
  pagination,
  onPaginationChange,
}: Pick<TasksPaginationProps, "pagination" | "onPaginationChange">) {
  return (
    <div className="flex items-center gap-2">
      <p className="text-sm font-medium whitespace-nowrap">Rows per page</p>
      <Select
        value={`${pagination.pageSize}`}
        onValueChange={(value) => {
          onPaginationChange((prev) => ({
            ...prev,
            pageIndex: 0,
            pageSize: Number(value),
          }));
        }}
      >
        <SelectTrigger
          data-testid="tasks-pagination-page-size"
          className="h-10 w-[76px] cursor-pointer sm:h-8"
        >
          <SelectValue placeholder={pagination.pageSize} />
        </SelectTrigger>
        <SelectContent side="top">
          {[10, 25, 50].map((pageSize) => (
            <SelectItem key={pageSize} value={`${pageSize}`} className="cursor-pointer">
              {pageSize}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function PageButtons({
  pages,
  currentPage,
  onPageChange,
}: {
  pages: PageNumber[];
  currentPage: number;
  onPageChange: (page: number) => void;
}) {
  return (
    <>
      {pages.map((page, index) =>
        page === "ellipsis" ? (
          <PaginationItem key={`ellipsis-${index}`}>
            <PaginationEllipsis />
          </PaginationItem>
        ) : (
          <PaginationItem key={page}>
            <Button
              variant={page === currentPage ? "outline" : "ghost"}
              size="icon"
              className="cursor-pointer"
              aria-current={page === currentPage ? "page" : undefined}
              onClick={() => onPageChange(page)}
            >
              {page}
            </Button>
          </PaginationItem>
        ),
      )}
    </>
  );
}

function PageNavigationButton({
  label,
  disabled,
  className,
  onClick,
  icon,
}: {
  label: string;
  disabled: boolean;
  className?: string;
  onClick: () => void;
  icon: ReactNode;
}) {
  return (
    <PaginationItem>
      <Button
        variant="ghost"
        size="icon"
        className={`cursor-pointer ${className ?? ""}`}
        disabled={disabled}
        onClick={onClick}
      >
        <span className="sr-only">{label}</span>
        {icon}
      </Button>
    </PaginationItem>
  );
}

function getPageNumbers(currentPage: number, pageCount: number): PageNumber[] {
  if (pageCount <= 5) {
    return Array.from({ length: pageCount }, (_, index) => index + 1);
  }

  const pages: PageNumber[] = [1];
  if (currentPage > 3) pages.push("ellipsis");

  const start = Math.max(2, currentPage - 1);
  const end = Math.min(pageCount - 1, currentPage + 1);
  for (let page = start; page <= end; page += 1) pages.push(page);

  if (currentPage < pageCount - 2) pages.push("ellipsis");
  pages.push(pageCount);
  return pages;
}
