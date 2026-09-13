"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type JiraActionBarProps = {
  workspaceId: string;
  testing: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

export function JiraActionBar({
  workspaceId,
  testing,
  loading,
  hasConfig,
  disableTest,
  onTest,
  onDelete,
}: JiraActionBarProps) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const removeConfirmation = t("jira:removeJiraConfiguration");
  const removeLabel = t("jira:removeConfiguration");

  useEffect(() => {
    if (!hasConfig && confirmingDelete) setConfirmingDelete(false);
  }, [confirmingDelete, hasConfig]);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? t("jira:pasteATokenToTestTheConnection") : undefined}
        data-testid="jira-test-button"
      >
        {testing ? t("jira:testingConnection") : t("jira:testConnection")}
      </Button>
      {hasConfig && (isMobile || isFinePointer || !confirmingDelete) && (
        <Button
          ref={deleteAnchorRef}
          type="button"
          variant="destructive"
          onClick={() => setConfirmingDelete(true)}
          className="ml-auto cursor-pointer"
          data-testid="jira-delete-button"
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
        confirmTestId="jira-remove-confirm"
        onOpenChange={setConfirmingDelete}
        focusReturnRef={deleteAnchorRef}
        onConfirm={onDelete}
        fallback={
          !isFinePointer ? (
            <InlineConfirmActions
              density="touch"
              testId="jira-remove-inline-confirmation"
              ariaLabel={removeConfirmation}
              description={removeConfirmation}
              cancelLabel={t("common:cancel")}
              confirmLabel={removeLabel}
              confirmAriaLabel={removeConfirmation}
              confirmTestId="jira-remove-confirm"
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
              confirmTestId="jira-remove-confirm"
              testId="jira-remove-confirm-popover"
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
