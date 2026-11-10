"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type ActionBarProps = {
  workspaceId: string;
  testing: boolean;
  loading: boolean;
  mutationPending: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

export function LinearActionBar({
  workspaceId,
  testing,
  loading,
  mutationPending,
  hasConfig,
  disableTest,
  onTest,
  onDelete,
}: ActionBarProps) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const removeConfirmation = t("linear:removeLinearConfigurationConfirm");
  const removeLabel = t("linear:removeConfiguration");

  useEffect(() => {
    if (!hasConfig && confirmingDelete) setConfirmingDelete(false);
  }, [confirmingDelete, hasConfig]);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest || mutationPending}
        className="cursor-pointer"
        title={disableTest ? t("linear:pasteAnApiKeyToTest") : undefined}
        data-testid="linear-test-button"
      >
        {testing ? t("linear:testing") : t("linear:testConnection")}
      </Button>
      {hasConfig && (isMobile || isFinePointer || !confirmingDelete) && (
        <Button
          ref={deleteAnchorRef}
          type="button"
          variant="destructive"
          onClick={() => setConfirmingDelete(true)}
          disabled={loading || mutationPending}
          className="ml-auto cursor-pointer"
          data-testid="linear-delete-button"
        >
          {removeLabel}
        </Button>
      )}
      <MobileActionConfirmation
        open={hasConfig && confirmingDelete}
        targetKey={workspaceId}
        title={removeConfirmation}
        cancelLabel={t("common:cancel")}
        confirmLabel={removeLabel}
        confirmAriaLabel={removeConfirmation}
        confirmDisabled={mutationPending}
        confirmTestId="linear-remove-confirm"
        onOpenChange={setConfirmingDelete}
        focusReturnRef={deleteAnchorRef}
        onConfirm={onDelete}
        fallback={
          !isFinePointer ? (
            <InlineConfirmActions
              density="touch"
              testId="linear-remove-inline-confirmation"
              ariaLabel={removeConfirmation}
              description={removeConfirmation}
              cancelLabel={t("common:cancel")}
              confirmLabel={removeLabel}
              confirmAriaLabel={removeConfirmation}
              confirmDisabled={mutationPending}
              confirmTestId="linear-remove-confirm"
              onCancel={() => setConfirmingDelete(false)}
              onClose={() => setConfirmingDelete(false)}
              onConfirm={onDelete}
            />
          ) : (
            <ActionConfirmPopover
              open={confirmingDelete}
              anchorRef={deleteAnchorRef}
              title={removeConfirmation}
              cancelLabel={t("common:cancel")}
              confirmLabel={removeLabel}
              confirmAriaLabel={removeConfirmation}
              confirmDisabled={mutationPending}
              confirmTestId="linear-remove-confirm"
              testId="linear-remove-confirm-popover"
              onOpenChange={setConfirmingDelete}
              onCancel={() => setConfirmingDelete(false)}
              onConfirm={onDelete}
            />
          )
        }
      />
    </div>
  );
}
