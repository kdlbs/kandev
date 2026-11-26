"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type RepoSyncRemovalActionsProps = {
  testIdPrefix: string;
  description: string;
  targetKey: string;
  subject: string;
  hasConfig: boolean;
  confirming: boolean;
  saving: boolean;
  onRequest: () => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

export function RepoSyncRemovalActions({
  testIdPrefix,
  description,
  targetKey,
  subject,
  hasConfig,
  confirming,
  saving,
  onRequest,
  onCancel,
  onConfirm,
}: RepoSyncRemovalActionsProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const anchorRef = useRef<HTMLButtonElement>(null);
  if (!hasConfig) return null;

  const removeLabel = t("workflows:remove");
  const fallback = (
    <InlineConfirmActions
      density="touch"
      testId={`${testIdPrefix}-remove-confirmation`}
      ariaLabel={removeLabel}
      description={description}
      cancelLabel={t("common:cancel")}
      confirmLabel={removeLabel}
      confirmAriaLabel={removeLabel}
      confirmTestId={`${testIdPrefix}-remove-confirm`}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );

  return (
    <>
      {(isMobile || !confirming) && (
        <Button
          ref={anchorRef}
          type="button"
          variant="destructive"
          onClick={onRequest}
          disabled={saving}
          className="sm:mr-auto cursor-pointer"
          data-testid={`${testIdPrefix}-remove`}
        >
          {removeLabel}
        </Button>
      )}
      <MobileActionConfirmation
        open={confirming}
        targetKey={targetKey}
        title={removeLabel}
        subject={subject}
        description={description}
        cancelLabel={t("common:cancel")}
        confirmLabel={removeLabel}
        confirmAriaLabel={removeLabel}
        confirmTestId={`${testIdPrefix}-remove-confirm`}
        testId={`${testIdPrefix}-remove-confirmation`}
        onOpenChange={(open) => {
          if (!open) onCancel();
        }}
        onConfirm={onConfirm}
        completionPolicy="await-with-retry"
        focusReturnRef={anchorRef}
        fallback={fallback}
      />
    </>
  );
}
