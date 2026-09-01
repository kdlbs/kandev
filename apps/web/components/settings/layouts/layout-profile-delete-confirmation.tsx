"use client";

import type { RefObject } from "react";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import type { SavedLayout } from "@/lib/types/http";

type LayoutProfileDeleteConfirmationProps = {
  profile: SavedLayout;
  isFinePointer: boolean;
  open: boolean;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function LayoutProfileDeleteConfirmation({
  profile,
  isFinePointer,
  open,
  anchorRef,
  onOpenChange,
  onConfirm,
}: LayoutProfileDeleteConfirmationProps) {
  const { t } = useTranslation();
  const title = t("settings:deleteLayoutProfileNamed", { name: profile.name });
  const description = profile.is_default
    ? t("settings:theBuiltInDefaultLayoutWill")
    : t("settings:thisProfileWillBeRemovedWhen");
  const cancelInlineDelete = () => {
    onOpenChange(false);
    queueMicrotask(() => anchorRef.current?.focus());
  };

  if (!isFinePointer) {
    if (!open)
      return <DeleteProfileButton anchorRef={anchorRef} onClick={() => onOpenChange(true)} />;
    return (
      <InlineConfirmActions
        density="touch"
        testId="layout-profile-delete-inline-confirmation"
        ariaLabel={title}
        description={description}
        cancelLabel={t("settings:cancel")}
        confirmLabel={t("settings:delete")}
        confirmAriaLabel={title}
        confirmTestId="layout-profile-delete-confirm"
        onCancel={cancelInlineDelete}
        onClose={() => onOpenChange(false)}
        onConfirm={onConfirm}
      />
    );
  }

  return (
    <>
      <DeleteProfileButton anchorRef={anchorRef} onClick={() => onOpenChange(true)} />
      <ActionConfirmPopover
        open={open}
        anchorRef={anchorRef}
        title={title}
        description={description}
        cancelLabel={t("settings:cancel")}
        confirmLabel={t("settings:delete")}
        confirmAriaLabel={title}
        confirmTestId="layout-profile-delete-confirm"
        testId="layout-profile-delete-confirm-popover"
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />
    </>
  );
}

function DeleteProfileButton({
  anchorRef,
  onClick,
}: {
  anchorRef: RefObject<HTMLButtonElement | null>;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          ref={anchorRef}
          type="button"
          size="icon-sm"
          variant="outline"
          className="min-h-11 min-w-11 cursor-pointer sm:min-h-8 sm:min-w-8"
          aria-label={t("settings:deleteLayoutProfile")}
          onClick={onClick}
          data-testid="layout-profile-delete"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("settings:deleteThisCustomLayoutAfterConfirmation")}</TooltipContent>
    </Tooltip>
  );
}
