"use client";

import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogClose, DialogContent, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSessionExecution } from "@/hooks/domains/session/use-session-execution";
import { SessionExecutionBody } from "./session-execution-body";

export function SessionExecutionDetails({
  sessionId,
  onCloseAutoFocus,
}: {
  sessionId: string;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const { session } = useSessionExecution(sessionId);
  return (
    <DialogContent
      showCloseButton={false}
      onCloseAutoFocus={onCloseAutoFocus}
      enterConfirms={false}
      className={
        isMobile
          ? "inset-0 h-dvh max-h-dvh w-full max-w-none sm:max-w-none translate-x-0 translate-y-0 rounded-none border-0 flex flex-col overflow-hidden pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
          : "flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-xl"
      }
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <ExecutionDetailsHeader name={session?.name || sessionId} isMobile={isMobile} />
      <div
        className="min-h-0 flex-1 space-y-6 overflow-y-auto overscroll-contain px-1 py-2 text-xs"
        data-testid="execution-details-body"
      >
        <SessionExecutionBody sessionId={sessionId} />
      </div>
    </DialogContent>
  );
}

function ExecutionDetailsHeader({ name, isMobile }: { name: string; isMobile: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-3">
      {isMobile && (
        <DialogClose asChild>
          <Button type="button" variant="ghost" className="h-12 min-h-12 px-3 cursor-pointer">
            <IconArrowLeft aria-hidden="true" className="size-4" />
            {t("common:back")}
          </Button>
        </DialogClose>
      )}
      <div className="min-w-0 flex-1">
        <DialogTitle>{t("agents:executionDetails")}</DialogTitle>
        <DialogDescription className="break-words">{name}</DialogDescription>
      </div>
      {!isMobile && (
        <DialogClose asChild>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common:close")}>
            <IconX aria-hidden="true" className="size-4" />
          </Button>
        </DialogClose>
      )}
    </div>
  );
}
