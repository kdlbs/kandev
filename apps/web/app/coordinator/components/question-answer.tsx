"use client";

import { useCallback, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import { ClarificationPanelSection } from "@/components/task/chat/clarification-panel-section";
import type { ClarificationOutcome } from "@/hooks/domains/session/use-clarification-group";
import type { RelayClarification } from "@/lib/api/domains/coordinator-relay-api";
import { rowPrimaryText } from "@/lib/needs-you-inbox/row-presentation";

export type QuestionAnswerProps = {
  bundle: RelayClarification;
  /** The item's root element, so the panel's keyboard shortcuts stay inside the item. */
  itemRef: RefObject<HTMLElement | null>;
  /** First keystroke or choice in the panel. */
  onEngaged: () => void;
  /** A recorded, lost or no-longer-active outcome: the item collapses. */
  onOutcome: () => void;
};

/**
 * The expanded question card: the shared clarification panel with the Inbox
 * row's outcome copy
 * (docs/specs/coordinator/system-design/relay.md "Question card").
 */
export function QuestionAnswer({ bundle, itemRef, onEngaged, onOutcome }: QuestionAnswerProps) {
  const { t } = useTranslation();
  const question = rowPrimaryText(bundle, t("needsYouInbox:questionFromAgent"));

  const handleOutcome = useCallback(
    (outcome: ClarificationOutcome) => {
      if (outcome.kind === "resolved") {
        if (!outcome.claimedByThisCaller) {
          const key =
            outcome.status === "rejected"
              ? "needsYouInbox:anotherCallerRejected"
              : "needsYouInbox:anotherCallerResolved";
          toast(t(key, { question }));
        }
        onOutcome();
        return;
      }
      if (outcome.kind === "no_longer_active") {
        toast(t("needsYouInbox:bundleNoLongerActive", { question }));
        onOutcome();
      }
    },
    [onOutcome, question, t],
  );

  const engage = useCallback(
    (event: { target: EventTarget }) => {
      const target = event.target as HTMLElement;
      if (target.closest?.('[data-testid="clarification-collapse-toggle"]')) return;
      onEngaged();
    },
    [onEngaged],
  );

  return (
    <div
      data-testid="question-answer"
      onKeyDownCapture={engage}
      onInputCapture={engage}
      onClickCapture={engage}
    >
      <ClarificationPanelSection
        pending
        messages={bundle.messages}
        onResolved={() => {}}
        onOutcome={handleOutcome}
        shortcutScopeRef={itemRef}
        maxHeightVh={50}
      />
    </div>
  );
}
