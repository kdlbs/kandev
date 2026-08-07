"use client";

import { type Table } from "@tanstack/react-table";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
} from "@kandev/ui/pagination";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  IconChevronLeft,
  IconChevronRight,
  IconChevronsLeft,
  IconChevronsRight,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

interface DataTablePaginationProps<TData> {
  table: Table<TData>;
}

function getPageNumbers(currentPage: number, pageCount: number): (number | "ellipsis")[] {
  const pages: (number | "ellipsis")[] = [];
  const showPages = 5;
  if (pageCount <= showPages) {
    for (let i = 1; i <= pageCount; i++) pages.push(i);
    return pages;
  }
  pages.push(1);
  if (currentPage > 3) pages.push("ellipsis");
  const start = Math.max(2, currentPage - 1);
  const end = Math.min(pageCount - 1, currentPage + 1);
  for (let i = start; i <= end; i++) pages.push(i);
  if (currentPage < pageCount - 2) pages.push("ellipsis");
  if (pageCount > 1) pages.push(pageCount);
  return pages;
}

function PageButtons({
  pages,
  currentPage,
  onPageChange,
}: {
  pages: (number | "ellipsis")[];
  currentPage: number;
  onPageChange: (page: number) => void;
}) {
  return (
    <>
      {pages.map((page, idx) =>
        page === "ellipsis" ? (
          <PaginationItem key={`ellipsis-${idx}`}>
            <PaginationEllipsis />
          </PaginationItem>
        ) : (
          <PaginationItem key={page}>
            <Button
              variant={page === currentPage ? "outline" : "ghost"}
              size="icon"
              className="cursor-pointer"
              onClick={() => onPageChange(page - 1)}
            >
              {page}
            </Button>
          </PaginationItem>
        ),
      )}
    </>
  );
}

export function DataTablePagination<TData>({ table }: DataTablePaginationProps<TData>) {
  const { t } = useTranslation();
  const currentPage = table.getState().pagination.pageIndex + 1;
  const pageCount = table.getPageCount();
  const pages = getPageNumbers(currentPage, pageCount);
  const pagination = table.getState().pagination;

  return (
    <div className="flex items-center justify-between px-2">
      <div className="flex-1 text-sm text-muted-foreground">
        {table.getFilteredRowModel().rows.length > 0 &&
          t("common:showingRange", {
            from: pagination.pageIndex * pagination.pageSize + 1,
            to: Math.min((pagination.pageIndex + 1) * pagination.pageSize, table.getRowCount()),
            count: table.getRowCount(),
          })}
      </div>
      <div className="flex items-center space-x-6 lg:space-x-8">
        <div className="flex items-center space-x-2">
          <p className="text-sm font-medium whitespace-nowrap">{t("common:rowsPerPage")}</p>
          <Select
            value={`${pagination.pageSize}`}
            onValueChange={(value) => {
              table.setPageSize(Number(value));
            }}
          >
            <SelectTrigger className="h-8 w-[70px]">
              <SelectValue placeholder={pagination.pageSize} />
            </SelectTrigger>
            <SelectContent side="top">
              {[10, 25, 50].map((pageSize) => (
                <SelectItem key={pageSize} value={`${pageSize}`}>
                  {pageSize}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon"
                className="hidden lg:flex cursor-pointer"
                onClick={() => table.setPageIndex(0)}
                disabled={!table.getCanPreviousPage()}
              >
                <span className="sr-only">{t("common:goToFirstPage")}</span>
                <IconChevronsLeft className="h-4 w-4" />
              </Button>
            </PaginationItem>
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon"
                className="cursor-pointer"
                onClick={() => table.previousPage()}
                disabled={!table.getCanPreviousPage()}
              >
                <span className="sr-only">{t("common:goToPreviousPage")}</span>
                <IconChevronLeft className="h-4 w-4" />
              </Button>
            </PaginationItem>
            <PageButtons
              pages={pages}
              currentPage={currentPage}
              onPageChange={(p) => table.setPageIndex(p)}
            />
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon"
                className="cursor-pointer"
                onClick={() => table.nextPage()}
                disabled={!table.getCanNextPage()}
              >
                <span className="sr-only">{t("common:goToNextPage")}</span>
                <IconChevronRight className="h-4 w-4" />
              </Button>
            </PaginationItem>
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon"
                className="hidden lg:flex cursor-pointer"
                onClick={() => table.setPageIndex(table.getPageCount() - 1)}
                disabled={!table.getCanNextPage()}
              >
                <span className="sr-only">{t("common:goToLastPage")}</span>
                <IconChevronsRight className="h-4 w-4" />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  );
}
