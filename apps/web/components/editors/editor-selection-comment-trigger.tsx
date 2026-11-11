"use client";

import type { RefObject } from "react";
import { Button } from "@kandev/ui/button";
import { IconMessagePlus } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  SelectionActionDock,
  SelectionActionDockButton,
} from "@/components/shared/selection-action-dock";

const COMMENT_BUTTON_TEST_ID = "code-editor-comment-button";

/**
 * Comment trigger for code-editor selections. Fine pointers get a button beside
 * the selection; touch devices get a bottom dock so the native selection menu
 * cannot cover it.
 */
export function EditorSelectionCommentTrigger({
  touch,
  position,
  positioning,
  hostRef,
  onOpen,
  testId = COMMENT_BUTTON_TEST_ID,
}: {
  touch: boolean;
  position: { left: number; top: number };
  positioning: "fixed" | "absolute";
  hostRef?: RefObject<HTMLElement | null>;
  onOpen: () => void;
  testId?: string;
}) {
  const { t } = useTranslation();
  if (touch) {
    return (
      <SelectionActionDock hostRef={hostRef} label={t("task:selectionActions")}>
        <SelectionActionDockButton
          icon={IconMessagePlus}
          label={t("task:commentOnSelection")}
          text={t("editors:comment")}
          onActivate={onOpen}
          testId={testId}
        />
      </SelectionActionDock>
    );
  }
  return (
    <Button
      size="sm"
      variant="secondary"
      data-testid={testId}
      className={cn(
        "floating-comment-btn z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer",
        positioning,
      )}
      style={position}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={onOpen}
    >
      <IconMessagePlus className="h-3.5 w-3.5" />
      {t("editors:comment")}
    </Button>
  );
}
