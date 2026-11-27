"use client";

import type { RefObject } from "react";
import type { RelayItemState } from "@/hooks/domains/coordinator/use-relay-item";
import { PermissionAnswer } from "./permission-answer";
import { QuestionAnswer } from "./question-answer";

/** The expanded answer card of an item: a question or a permission, by the kind fixed at expansion. */
export function RelayAnswer({
  relay,
  itemRef,
}: {
  relay: RelayItemState;
  itemRef: RefObject<HTMLElement | null>;
}) {
  const rendered = relay.rendered;
  if (rendered?.kind === "clarification") {
    return (
      <QuestionAnswer
        bundle={rendered.bundle}
        itemRef={itemRef}
        onEngaged={relay.markEngaged}
        onOutcome={relay.finishOutcome}
      />
    );
  }
  if (rendered?.kind === "permission") {
    return (
      <PermissionAnswer
        permission={rendered.permission}
        onEngaged={relay.markEngaged}
        onDone={relay.finishOutcome}
      />
    );
  }
  return null;
}
