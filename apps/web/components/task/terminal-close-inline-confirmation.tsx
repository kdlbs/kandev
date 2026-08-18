"use client";

import { useEffect, useRef } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

type TerminalCloseInlineConfirmationProps = {
  density?: "compact" | "touch";
  testId?: string;
  onCancel: () => void;
  onConfirm: () => void;
};

export function TerminalCloseInlineConfirmation({
  density = "compact",
  testId = "terminal-menu-close-confirmation",
  onCancel,
  onConfirm,
}: TerminalCloseInlineConfirmationProps) {
  const { t } = useTranslation();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const touch = density === "touch";
  const actionClass = touch ? "h-11 min-w-11 px-2" : "h-10 min-w-10 px-2 text-xs";

  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  return (
    <div
      role="group"
      aria-label={t("task:closeTerminal")}
      data-testid={testId}
      className={`flex shrink-0 items-center justify-end gap-1 ${touch ? "min-h-11" : "w-full min-h-10"}`}
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        event.stopPropagation();
        onCancel();
      }}
    >
      <Button
        ref={cancelRef}
        type="button"
        variant="ghost"
        size="sm"
        className={`${actionClass} transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]`}
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        variant="destructive"
        size="sm"
        className={`${actionClass} transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]`}
        onClick={onConfirm}
      >
        {t("task:closeTerminal2")}
      </Button>
    </div>
  );
}
