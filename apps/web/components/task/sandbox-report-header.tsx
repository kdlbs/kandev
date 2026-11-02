"use client";

import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogClose, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function SandboxReportHeader({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const title = useRef<HTMLHeadingElement>(null);
  useLayoutEffect(() => title.current?.focus(), []);
  return (
    <div className="flex shrink-0 items-center gap-3">
      <Button
        type="button"
        variant="ghost"
        className="h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
        onClick={onBack}
      >
        <IconArrowLeft aria-hidden className="size-4" />
        {t("common:back")}
      </Button>
      <div className="min-w-0 flex-1">
        <DialogTitle ref={title} tabIndex={-1} className="focus:outline-none">
          {t("agents:reportTitle")}
        </DialogTitle>
        <DialogDescription>{t("agents:reportDescription")}</DialogDescription>
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
