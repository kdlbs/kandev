"use client";

import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { detectPlatform, formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";

type ComposerPromptSuggestionHintProps = {
  suggestion: string;
  /** The user's configured submit key, which also sends the suggestion. */
  submitKey: "enter" | "cmd_enter";
  onAccept: () => void;
};

/** Accept affordance for the composer's ghost-text suggestion: a Tab keycap on
 *  fine pointers, a 44px "Use reply" button on touch. */
export function ComposerPromptSuggestionHint({
  suggestion,
  submitKey,
  onAccept,
}: ComposerPromptSuggestionHintProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const submitShortcut = formatShortcut(
    submitKey === "enter" ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT,
    detectPlatform(),
  );
  return (
    <button
      type="button"
      data-testid="prompt-suggestion-accept"
      aria-label={t("task:promptSuggestionAcceptAria", { suggestion })}
      // Keep focus in the editor so the accepted text can be edited immediately.
      onMouseDown={(event) => event.preventDefault()}
      onClick={onAccept}
      className={cn(
        "absolute top-1.5 right-2 z-10 flex items-center gap-1.5 rounded border border-border/40",
        "bg-background/80 text-xs text-muted-foreground cursor-pointer hover:text-foreground",
        isFinePointer ? "h-6 px-2" : "min-h-11 px-3",
      )}
    >
      {isFinePointer ? (
        <>
          <kbd className="px-1 py-0.5 text-xs font-medium border border-border/40 rounded">
            {t("task:keyTab")}
          </kbd>
          <span>{t("task:promptSuggestionAccept")}</span>
          <span aria-hidden="true">·</span>
          <span>{t("task:promptSuggestionShortcutSends", { shortcut: submitShortcut })}</span>
        </>
      ) : (
        <span>{t("task:promptSuggestionUseReply")}</span>
      )}
    </button>
  );
}
