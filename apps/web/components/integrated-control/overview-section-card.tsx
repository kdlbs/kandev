"use client";

import type { ReactNode } from "react";
import { Card } from "@kandev/ui/card";
import { useTranslation } from "react-i18next";

/** A titled block on the overview: a heading bar, then whatever the section holds. */
export function SectionCard({
  id,
  title,
  action,
  surface = false,
  children,
}: {
  id?: string;
  title: string;
  action?: ReactNode;
  /** Set by the section that waits on a person, which is not another card. */
  surface?: boolean;
  children: ReactNode;
}) {
  return (
    <Card
      className={`scroll-mt-4 p-0 ${surface ? "border-attention-border bg-attention-surface" : ""}`}
      id={id}
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
        <h2
          className={`text-sm font-semibold ${surface ? "text-attention-title" : "text-foreground"}`}
        >
          {title}
        </h2>
        {action}
      </div>
      {children}
    </Card>
  );
}

/** The line a section shows when it has nothing to report. */
export function SectionCardEmpty() {
  const { t } = useTranslation();
  return (
    <div className="px-4 py-6 text-center text-sm text-muted-foreground">
      {t("office:allClear")}
    </div>
  );
}
