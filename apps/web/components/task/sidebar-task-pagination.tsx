"use client";

import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SidebarTaskPageResponse } from "@/lib/types/http";

export function SidebarTaskPagination({
  page,
  pending,
  error,
  onPageChange,
  onRetry,
  touchTargets = false,
}: {
  page: SidebarTaskPageResponse | null;
  pending: boolean;
  error: string | null;
  onPageChange: (page: number) => void;
  onRetry: () => void;
  touchTargets?: boolean;
}) {
  const { t } = useTranslation();
  if (!page || page.total_visible_tasks <= page.page_size) return null;
  const totalPages = Math.max(1, Math.ceil(page.total_visible_tasks / page.page_size));
  const buttonSize = touchTargets ? "min-h-11 min-w-11" : "h-7";

  return (
    <div className="space-y-1 border-t border-border px-2 py-2" data-testid="sidebar-page-controls">
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
      {error && (
        <div className="flex items-center justify-between gap-2 text-xs" role="alert">
          <span className="text-destructive">{t("sidebar:pageLoadFailed")}</span>
          <Button
            variant="link"
            size="sm"
            className={touchTargets ? "min-h-11" : "h-7"}
            onClick={onRetry}
          >
            {t("sidebar:retry")}
          </Button>
        </div>
      )}
    </div>
  );
}
