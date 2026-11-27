"use client";

import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
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
  if (page.total_visible_tasks <= page.page_size && !page.grouping_revision) return null;

  return (
    <div className="space-y-1 border-t border-border px-2 py-2" data-testid="sidebar-page-controls">
      <PageButtons
        page={page}
        pending={pending}
        onPageChange={onPageChange}
        touchTargets={touchTargets}
      />
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
  const buttonSize = touchTargets
    ? "min-h-[44px] min-w-[44px]"
    : controlSizingClassName(
        "standard",
        "max-md:min-w-[44px] [@media(any-pointer:coarse)]:min-w-[44px]",
      );
  return (
    <div className="flex items-center justify-between gap-2">
      {totalPages > 1 && (
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
      )}
      <span className="min-w-0 text-center text-xs text-muted-foreground" aria-live="polite">
        {t(page.grouping_revision ? "sidebar:pluginPageStatus" : "sidebar:pageStatus", {
          page: page.page,
          total: totalPages,
          ...(page.grouping_revision ? { count: page.total_tasks } : {}),
        })}
      </span>
      {totalPages > 1 && (
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
      )}
    </div>
  );
}
