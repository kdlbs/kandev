"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import type { RepositorySet } from "@/lib/types/http";

type RepositorySetDeleteConfirmationProps = {
  set: RepositorySet;
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Fine-pointer confirmation for deleting one repository set. The copy states
 * plainly that no repository is affected, because "delete set" next to a list
 * of repository names reads as though it might remove them.
 */
export function RepositorySetDeleteConfirmation({
  set,
  open,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
}: RepositorySetDeleteConfirmationProps) {
  const { t } = useTranslation();

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={t("workspaces:repositorySetsDeleteTitle", { name: set.name })}
      description={t("workspaces:repositorySetsDeleteDescription")}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("workspaces:repositorySetsDelete")}
      confirmAriaLabel={t("workspaces:repositorySetsDeleteTitle", { name: set.name })}
      confirmTestId="repository-set-delete-confirm"
      testId="repository-set-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
