"use client";

import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

type SessionTabCloseActionProps = {
  sessionId: string | undefined;
  onClose: () => void;
};

export function SessionTabCloseAction({ sessionId, onClose }: SessionTabCloseActionProps) {
  const { t } = useTranslation();
  if (!sessionId) return null;

  return (
    <button
      type="button"
      className="session-tab-close-action dv-default-tab-action inline-flex h-4 w-4 shrink-0 cursor-pointer items-center justify-center rounded p-0 text-muted-foreground"
      data-testid={`session-tab-close-${sessionId}`}
      aria-label={t("common:close")}
      onPointerDown={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }}
    >
      <IconX className="h-3 w-3" aria-hidden="true" />
    </button>
  );
}
