"use client";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import type { useSandboxRestartReview } from "@/hooks/domains/session/use-sandbox-restart-review";
type Model = ReturnType<typeof useSandboxRestartReview>;
export function RestartReviewHeader({ model, isMobile }: { model: Model; isMobile: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-3">
      {isMobile && (
        <Button type="button" variant="ghost" className="h-12 min-h-12" onClick={model.cancel}>
          <IconArrowLeft aria-hidden="true" className="size-4" />
          {t("common:back")}
        </Button>
      )}
      <div className="min-w-0 flex-1">
        <DialogTitle>{t("agents:restartReviewTitle")}</DialogTitle>
        <DialogDescription className="break-words">{model.name}</DialogDescription>
      </div>
      {!isMobile && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("common:close")}
          onClick={model.cancel}
        >
          <IconX aria-hidden="true" className="size-4" />
        </Button>
      )}
    </div>
  );
}
export function RestartReviewActions({ model }: { model: Model }) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 flex-col gap-2 border-t pt-3 md:flex-row md:flex-wrap md:justify-end">
      <Button
        type="button"
        variant="outline"
        className="h-12 min-h-12 whitespace-normal md:h-7 md:min-h-7 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-h-12"
        disabled={!model.canUseCurrent || model.busy}
        onClick={() => void model.confirmCurrent()}
      >
        {t(model.busy ? "common:loading" : "agents:restartUseCurrent")}
      </Button>
      <Button
        type="button"
        className="h-12 min-h-12 whitespace-normal md:h-7 md:min-h-7 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-h-12"
        disabled={!model.valid || model.busy}
        onClick={model.confirmCaptured}
      >
        {t("agents:restartKeepCaptured")}
      </Button>
    </div>
  );
}
