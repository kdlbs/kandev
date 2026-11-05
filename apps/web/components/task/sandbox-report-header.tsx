"use client";

import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogClose, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export const sandboxReportActionClass =
  "h-7 min-h-7 max-md:min-h-12 pointer-coarse:min-h-12 cursor-pointer";

export function SandboxReportHeader({
  onBack,
  title: titleText,
  description,
}: {
  onBack: () => void;
  title?: string;
  description?: string;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const title = useRef<HTMLHeadingElement>(null);
  useLayoutEffect(() => title.current?.focus(), []);
  return (
    <div className="flex shrink-0 items-center gap-3">
      <Button type="button" variant="ghost" className={sandboxReportActionClass} onClick={onBack}>
        <IconArrowLeft aria-hidden className="size-4" />
        {t("common:back")}
      </Button>
      <div className="min-w-0 flex-1">
        <DialogTitle ref={title} tabIndex={-1} className="focus:outline-none">
          {titleText ?? t("agents:reportTitle")}
        </DialogTitle>
        <DialogDescription>{description ?? t("agents:reportDescription")}</DialogDescription>
      </div>
      {!isMobile && (
        <DialogClose asChild>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common:close")}>
            <IconX aria-hidden className="size-4" />
          </Button>
        </DialogClose>
      )}
    </div>
  );
}
