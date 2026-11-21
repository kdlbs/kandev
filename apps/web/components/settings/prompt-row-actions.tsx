"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconEdit, IconEye, IconEyeOff, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { CustomPrompt } from "@/lib/types/http";

type PromptOpenButtonProps = {
  promptId: string;
  /** Edit opens the editable text; otherwise the eye opens it read-only. */
  editable: boolean;
  open: boolean;
  onToggle: () => void;
  disabled: boolean;
};

// PromptOpenButton shows or hides a row's text: Edit for text the user can
// change, the eye for the read-only repository version. Its accessible name
// stays stable; aria-expanded reports the state.
function PromptOpenButton({ promptId, editable, open, onToggle, disabled }: PromptOpenButtonProps) {
  const { t } = useTranslation();
  const ViewIcon = open ? IconEyeOff : IconEye;
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={onToggle}
      disabled={disabled}
      aria-label={editable ? t("settings:edit") : t("settings:promptViewContent")}
      aria-expanded={open}
      aria-controls={`prompt-body-${promptId}`}
      className="cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11"
      data-testid={editable ? "prompt-edit-button" : "prompt-view-button"}
    >
      {editable ? <IconEdit className="h-4 w-4" /> : <ViewIcon className="h-4 w-4" />}
    </Button>
  );
}

type PromptRowActionsProps = {
  prompt: CustomPrompt;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  editable: boolean;
  open: boolean;
  onToggleOpen: (prompt: CustomPrompt) => void;
  /** Prompt Sync owns a synced prompt, so it has no delete. */
  canDelete: boolean;
  onOpenDelete: (prompt: CustomPrompt) => void;
  isBusy: boolean;
  showCreate: boolean;
  isFinePointer: boolean;
  isDeleteTarget: boolean;
};

export function PromptRowActions({
  prompt,
  deleteAnchorRef,
  editable,
  open,
  onToggleOpen,
  canDelete,
  onOpenDelete,
  isBusy,
  showCreate,
  isFinePointer,
  isDeleteTarget,
}: PromptRowActionsProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  // A coarse-pointer delete confirmation replaces the row actions inline.
  if (!isMobile && !isFinePointer && isDeleteTarget) return null;
  return (
    <div className="flex items-center gap-1">
      <PromptOpenButton
        promptId={prompt.id}
        editable={editable}
        open={open}
        onToggle={() => onToggleOpen(prompt)}
        disabled={isBusy || showCreate}
      />
      {canDelete && (
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
      )}
    </div>
  );
}
