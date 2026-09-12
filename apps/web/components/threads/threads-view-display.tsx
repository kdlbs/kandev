"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { ThreadLayout, ThreadView } from "@/lib/state/slices/ui/thread-view-types";
import { SectionLabel } from "./threads-view-editor-actions";

type LayoutProps = {
  value: ThreadLayout;
  onChange: (layout: ThreadLayout) => void;
  mobile?: boolean;
};

function ThreadsLayoutSelect({ value, onChange, mobile = false }: LayoutProps) {
  const { t } = useTranslation("threads");
  return (
    <Select
      value={value}
      onValueChange={(next) => {
        if (next !== "grid" && next !== "columns") return;
        onChange(next);
      }}
    >
      <SelectTrigger
        id="threads-layout"
        className={`${mobile ? "h-11" : "h-7"} min-w-0 w-28 max-w-[45%] text-xs`}
        aria-label={t("layout")}
        data-testid="threads-layout-select"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="columns" className={mobile ? "min-h-11" : undefined}>
          {t("layoutColumns")}
        </SelectItem>
        <SelectItem value="grid" className={mobile ? "min-h-11" : undefined}>
          {t("layoutGrid")}
        </SelectItem>
      </SelectContent>
    </Select>
  );
}

export function ThreadsViewDisplay({
  current,
  mobile,
  gridHeightFallback,
  onUpdate,
  children,
}: {
  current: Pick<ThreadView, "layout" | "autoHideComposer">;
  mobile: boolean;
  gridHeightFallback: boolean;
  onUpdate: (patch: Partial<Pick<ThreadView, "layout" | "autoHideComposer">>) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation("threads");
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  return (
    <section className="space-y-3 border-b p-2" aria-label={t("display")}>
      <SectionLabel>{t("display")}</SectionLabel>
      <div className="space-y-1">
        <div className="flex items-center justify-between gap-3">
          <label htmlFor="threads-layout" className="text-xs font-medium">
            {t("layout")}
          </label>
          <ThreadsLayoutSelect
            value={current.layout}
            mobile={mobile}
            onChange={(layout) => onUpdate({ layout })}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {current.layout === "grid" ? t("layoutGridDescription") : t("layoutColumnsDescription")}
        </p>
        {isMobile && (
          <p className="text-xs text-muted-foreground" data-testid="threads-phone-layout-hint">
            {t("phoneLayoutHint")}
          </p>
        )}
        {gridHeightFallback && current.layout === "grid" && (
          <p role="status" className="text-xs text-muted-foreground">
            {t("gridHeightFallback")}
          </p>
        )}
      </div>
      <div className="space-y-1">
        <label
          htmlFor="threads-auto-hide-composer"
          className={`flex ${mobile ? "min-h-11" : "min-h-7"} cursor-pointer items-center justify-between gap-4 text-xs font-medium`}
          data-testid="threads-auto-hide-row"
        >
          <span>{t("autoHideComposer")}</span>
          <Switch
            id="threads-auto-hide-composer"
            checked={current.autoHideComposer}
            onCheckedChange={(autoHideComposer) => onUpdate({ autoHideComposer })}
            className={`after:-inset-x-2${mobile ? " after:-inset-y-[14px]" : ""}`}
            aria-describedby="threads-auto-hide-description"
          />
        </label>
        <p id="threads-auto-hide-description" className="text-xs text-muted-foreground">
          {t("autoHideComposerDescription")}
        </p>
        {(isMobile || !isFinePointer) && (
          <p className="text-xs text-muted-foreground" data-testid="threads-touch-composer-hint">
            {t("touchComposerHint")}
          </p>
        )}
      </div>
      <div className="space-y-1">
        {children}
        <p className="text-xs text-muted-foreground">{t("maxChatsDescription")}</p>
      </div>
    </section>
  );
}
