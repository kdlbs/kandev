"use client";

import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";

export type SessionTabCloseActionState = {
  isBusy: boolean;
  isDisabled: boolean;
  showSpinner: boolean;
};

export function getSessionTabCloseActionState(isDeleting: boolean): SessionTabCloseActionState {
  return {
    isBusy: isDeleting,
    isDisabled: isDeleting,
    showSpinner: isDeleting,
  };
}

type SessionTabCloseActionProps = {
  sessionId: string | undefined;
  isDeleting: boolean;
  onClose: () => void;
};

export function SessionTabCloseAction({
  sessionId,
  isDeleting,
  onClose,
}: SessionTabCloseActionProps) {
  const { t } = useTranslation();
  if (!sessionId) return null;

  const state = getSessionTabCloseActionState(isDeleting);

  return (
    <button
      type="button"
      className="session-tab-close-action dv-default-tab-action inline-flex h-4 w-4 shrink-0 items-center justify-center rounded p-0 text-muted-foreground"
      data-testid={`session-tab-close-${sessionId}`}
      aria-label={t("common:deleteSession")}
      aria-busy={state.isBusy}
      disabled={state.isDisabled}
      onPointerDown={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        if (!state.isDisabled) onClose();
      }}
    >
      {state.showSpinner ? (
        <GridSpinner className="h-3 w-3" />
      ) : (
        <IconX className="h-3 w-3" aria-hidden="true" />
      )}
    </button>
  );
}
