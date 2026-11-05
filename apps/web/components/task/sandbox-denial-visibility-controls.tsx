"use client";

import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SandboxDenialVisibility } from "@/hooks/domains/session/use-sandbox-denial-visibility";

export function SandboxDenialVisibilityControls({
  visibility,
  selectedIds,
  onHide,
  onShow,
}: {
  visibility: SandboxDenialVisibility;
  selectedIds: string[];
  onHide: () => void;
  onShow: () => void;
}) {
  const { t } = useTranslation();
  const show = useRef<HTMLButtonElement>(null);
  const focusAfterHide = useRef(false);
  useLayoutEffect(() => {
    if (focusAfterHide.current && visibility.hiddenCount > 0) {
      show.current?.focus();
      focusAfterHide.current = false;
    }
  }, [visibility.hiddenCount]);
  const controlClass =
    "h-auto min-h-7 max-w-full whitespace-normal max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer";
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={controlClass}
          disabled={selectedIds.length === 0}
          onClick={() => {
            focusAfterHide.current = true;
            visibility.hide(selectedIds);
            onHide();
          }}
        >
          {t("agents:reportHideSelected")}
        </Button>
        <Button
          ref={show}
          type="button"
          variant="outline"
          size="sm"
          className={controlClass}
          disabled={visibility.hiddenCount === 0}
          onClick={() => {
            onShow();
            visibility.showAll();
          }}
        >
          {t("agents:reportShowHidden")}
        </Button>
      </div>
      <p className="text-muted-foreground">{t("agents:reportHideHelp")}</p>
      {visibility.hiddenCount > 0 && (
        <p role="status">{t("agents:reportHidden", { count: visibility.hiddenCount })}</p>
      )}
    </div>
  );
}
