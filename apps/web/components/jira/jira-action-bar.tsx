"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type JiraActionBarProps = {
  testing: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

export function JiraActionBar({
  testing,
  loading,
  hasConfig,
  disableTest,
  onTest,
  onDelete,
}: JiraActionBarProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const removeConfirmation = t("jira:removeJiraConfiguration");

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
      {hasConfig && (isFinePointer || !confirmingDelete) && (
        <Button
          ref={deleteAnchorRef}
          type="button"
          variant="destructive"
          onClick={() => setConfirmingDelete(true)}
          className="ml-auto min-h-11 cursor-pointer"
          data-testid="jira-delete-button"
        >
          {t("jira:removeConfiguration")}
        </Button>
      )}
      {hasConfig && !isFinePointer && confirmingDelete ? (
        <InlineConfirmActions
          density="touch"
          testId="jira-remove-inline-confirmation"
          ariaLabel={removeConfirmation}
          description={removeConfirmation}
          cancelLabel={t("common:cancel")}
          confirmLabel={t("jira:removeConfiguration")}
          confirmAriaLabel={removeConfirmation}
          confirmTestId="jira-remove-confirm"
          onCancel={() => setConfirmingDelete(false)}
          onClose={() => setConfirmingDelete(false)}
          onConfirm={onDelete}
        />
      ) : null}
      {hasConfig && isFinePointer ? (
        <ActionConfirmPopover
          open={confirmingDelete}
          anchorRef={deleteAnchorRef}
          title={removeConfirmation}
          cancelLabel={t("common:cancel")}
          confirmLabel={t("jira:removeConfiguration")}
          confirmAriaLabel={removeConfirmation}
          confirmTestId="jira-remove-confirm"
          testId="jira-remove-confirm-popover"
          onOpenChange={setConfirmingDelete}
          onCancel={() => setConfirmingDelete(false)}
          onConfirm={onDelete}
        />
      ) : null}
    </div>
  );
}
