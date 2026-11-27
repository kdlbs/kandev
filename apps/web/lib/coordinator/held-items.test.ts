import { describe, it } from "vitest";
import assert from "node:assert/strict";
import type { NeedsYouItem, NeedsYouQuestionItem } from "./attention";
import { mergeHeldNeedsYouItems } from "./held-items";

function question(id: string): NeedsYouQuestionItem {
  return {
    id,
    kind: "question",
    task: { id, title: "question" },
    pendingAction: "clarification",
    referenceTimeMs: undefined,
    ageMs: undefined,
  };
}

describe("held Needs you items", () => {
  it("preserves the expanded snapshot and its position when the same task changes kind", () => {
    const held = question("task");
    const live: NeedsYouItem = {
      ...held,
      kind: "stall",
      stall: { task_id: "task", stalled_for_ms: 100, last_event_at: "", detected_at: "" },
    };
    const other = question("other");
    const result = mergeHeldNeedsYouItems([other, live], { task: { item: held, index: 0 } });
    assert.deepEqual(result, [held, other]);
    assert.equal(result[0], held);
  });

  it("retains disappeared held items and restores current data when released", () => {
    const held = question("held");
    const other = question("other");
    assert.deepEqual(mergeHeldNeedsYouItems([other], { held: { item: held, index: 1 } }), [
      other,
      held,
    ]);
    assert.deepEqual(mergeHeldNeedsYouItems([other], {}), [other]);
  });
});
