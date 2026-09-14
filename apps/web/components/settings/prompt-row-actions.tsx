"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconEdit, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { CustomPrompt } from "@/lib/types/http";

type PromptRowActionsProps = {
  prompt: CustomPrompt;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onStartEditing: (prompt: CustomPrompt) => void;
  onOpenDelete: (prompt: CustomPrompt) => void;
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
  isBusy,
  showCreate,
  isFinePointer,
  isDeleteTarget,
}: PromptRowActionsProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  return (
    <div className="flex items-center gap-2">
      {isMobile || isFinePointer || !isDeleteTarget ? (
        <>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onStartEditing(prompt)}
            disabled={isBusy || showCreate}
            aria-label={t("settings:edit")}
            className="cursor-pointer"
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
            className="cursor-pointer"
            data-testid="prompt-delete-button"
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </>
      ) : null}
    </div>
  );
}
