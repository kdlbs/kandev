"use client";

import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@kandev/ui/pagination";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type IntegrationCursorPaginationProps = {
  page: number;
  itemCount: number;
  hasPrevious: boolean;
  hasNext: boolean;
  loading?: boolean;
  onPrevious: () => void;
  onNext: () => void;
  testId?: string;
};

export function IntegrationCursorPagination({
  page,
  itemCount,
  hasPrevious,
  hasNext,
  loading = false,
  onPrevious,
  onNext,
  testId,
}: IntegrationCursorPaginationProps) {
  const { t } = useTranslation("integrations");
  if (page <= 1 && !hasNext) return null;
  const previousDisabled = loading || !hasPrevious;
  const nextDisabled = loading || !hasNext;
  return (
    <div
      className="flex shrink-0 items-center justify-between border-t px-4 py-3 sm:px-6"
      data-testid={testId}
    >
      <div className="text-xs tabular-nums text-muted-foreground">
        {t("integrations:cursorPaginationSummary", { page, count: itemCount })}
      </div>
      <Pagination className="mx-0 w-auto justify-end">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(event) => {
                event.preventDefault();
                if (!previousDisabled) onPrevious();
              }}
              aria-disabled={previousDisabled}
              className={cn(
                "min-h-11 cursor-pointer sm:min-h-9",
                previousDisabled && "pointer-events-none opacity-50",
              )}
            />
          </PaginationItem>
          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(event) => {
                event.preventDefault();
                if (!nextDisabled) onNext();
              }}
              aria-disabled={nextDisabled}
              className={cn(
                "min-h-11 cursor-pointer sm:min-h-9",
                nextDisabled && "pointer-events-none opacity-50",
              )}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
