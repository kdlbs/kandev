"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconEdit, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

import { PromptDeleteConfirmation } from "@/components/settings/prompt-delete-confirmation";
import type { CustomPrompt } from "@/lib/types/http";

type PromptRowActionsProps = {
  prompt: CustomPrompt;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onStartEditing: (prompt: CustomPrompt) => void;
  onOpenDelete: (prompt: CustomPrompt) => void;
  onDeleteClose: () => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void;
  isBusy: boolean;
  showCreate: boolean;
  isFinePointer: boolean;
  isDeleteTarget: boolean;
};

export function PromptRowActions({
  prompt,
  deleteAnchorRef,
  onStartEditing,
  onOpenDelete,
  onDeleteClose,
  onDeleteCancel,
  onDeleteConfirm,
  isBusy,
  showCreate,
  isFinePointer,
  isDeleteTarget,
}: PromptRowActionsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      {isFinePointer || !isDeleteTarget ? (
        <>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onStartEditing(prompt)}
            disabled={isBusy || showCreate}
            aria-label={t("settings:edit")}
            className="min-h-11 min-w-11 cursor-pointer"
            data-testid="prompt-edit-button"
          >
            <IconEdit className="h-4 w-4" />
          </Button>
          <Button
            ref={deleteAnchorRef}
            variant="ghost"
            size="icon"
            onClick={() => onOpenDelete(prompt)}
            disabled={isBusy}
            aria-label={t("settings:promptDelete")}
            className="min-h-11 min-w-11 cursor-pointer"
            data-testid="prompt-delete-button"
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </>
      ) : null}
      {isFinePointer ? (
        <PromptDeleteConfirmation
          promptName={prompt.name}
          open={isDeleteTarget}
          isFinePointer={isFinePointer}
          anchorRef={deleteAnchorRef}
          isBusy={isBusy}
          onClose={onDeleteClose}
          onCancel={onDeleteCancel}
          onConfirm={onDeleteConfirm}
        />
      ) : null}
    </div>
  );
}
