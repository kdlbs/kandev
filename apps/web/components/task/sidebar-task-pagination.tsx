"use client";

import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SidebarTaskPageResponse } from "@/lib/types/http";

export function SidebarTaskPagination({
  page,
  pending,
  onPageChange,
  touchTargets = false,
}: {
  page: SidebarTaskPageResponse | null;
  pending: boolean;
  onPageChange: (page: number) => void;
  touchTargets?: boolean;
}) {
  if (!page) return null;
  const showPaging = page !== null && page.total_visible_tasks > page.page_size;
  if (!showPaging) return null;

  return (
    <div className="space-y-1 border-t border-border px-2 py-2" data-testid="sidebar-page-controls">
      {showPaging && page ? (
        <PageButtons
          page={page}
          pending={pending}
          onPageChange={onPageChange}
          touchTargets={touchTargets}
        />
      ) : null}
    </div>
  );
}

function PageButtons({
  page,
  pending,
  onPageChange,
  touchTargets,
}: {
  page: SidebarTaskPageResponse;
  pending: boolean;
  onPageChange: (page: number) => void;
  touchTargets: boolean;
}) {
  const { t } = useTranslation();
  const totalPages = Math.max(1, Math.ceil(page.total_visible_tasks / page.page_size));
  const buttonSize = touchTargets ? "min-h-11 min-w-11" : "h-7";
  return (
    <div className="flex items-center justify-between gap-2">
      <Button
        variant="ghost"
        size="sm"
        className={buttonSize}
        aria-label={t("sidebar:previousPage")}
        disabled={pending || !page.has_previous}
        onClick={() => onPageChange(page.page - 1)}
      >
        <IconChevronLeft className="h-4 w-4" />
        <span className={touchTargets ? "" : "sr-only"}>{t("sidebar:previousPage")}</span>
      </Button>
      <span className="text-xs text-muted-foreground" aria-live="polite">
        {t("sidebar:pageStatus", { page: page.page, total: totalPages })}
      </span>
      <Button
        variant="ghost"
        size="sm"
        className={buttonSize}
        aria-label={t("sidebar:nextPage")}
        disabled={pending || !page.has_next}
        onClick={() => onPageChange(page.page + 1)}
      >
        <span className={touchTargets ? "" : "sr-only"}>{t("sidebar:nextPage")}</span>
        <IconChevronRight className="h-4 w-4" />
      </Button>
    </div>
  );
}
