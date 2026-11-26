import { describe, expect, it } from "vitest";
import type { CoordinatorSettings } from "@/lib/api/domains/coordinator-api";
import {
  buildPutRequest,
  draftFromSettings,
  isPolicyDirty,
  mergeStored,
  settleAfterSave,
} from "./control-draft";

const SIX = {
  create_task: "requires_approval",
  start_agent: "denied",
  message: "denied",
  move: "denied",
  resume: "denied",
  stop: "denied",
} as const;

function settings(actions: Record<string, string>): CoordinatorSettings {
  return {
    policy: { actions },
    watches: { scope: "all", workflow_ids: [] },
  } as unknown as CoordinatorSettings;
}

describe("answer class in the control draft", () => {
  it("an unnamed answer is not dirty and is not sent", () => {
    const stored = draftFromSettings(settings({ ...SIX }));
    expect(isPolicyDirty(stored, stored)).toBe(false);
    expect(buildPutRequest(stored, stored)).toEqual({});
  });

  it("changing answer is dirty and the PUT names every action including answer", () => {
    const stored = draftFromSettings(settings({ ...SIX, answer: "requires_approval" }));
    const draft = { ...stored, actions: { ...stored.actions, answer: "denied" as const } };
    expect(isPolicyDirty(draft, stored)).toBe(true);
    expect(buildPutRequest(draft, stored).policy?.actions.answer).toBe("denied");
  });

  it("a re-read keeps a drafted answer edit and follows stored changes elsewhere", () => {
    const oldStored = draftFromSettings(settings({ ...SIX, answer: "requires_approval" }));
    const draft = { ...oldStored, actions: { ...oldStored.actions, answer: "denied" as const } };
    const newStored = draftFromSettings(
      settings({ ...SIX, move: "requires_approval", answer: "requires_approval" }),
    );
    const merged = mergeStored(draft, oldStored, newStored);
    expect(merged.actions.answer).toBe("denied");
    expect(merged.actions.move).toBe("requires_approval");
  });

  it("settleAfterSave keeps an answer edit made while the save was in flight", () => {
    const sent = draftFromSettings(settings({ ...SIX, answer: "requires_approval" }));
    const current = { ...sent, actions: { ...sent.actions, answer: "denied" as const } };
    const response = draftFromSettings(settings({ ...SIX, answer: "requires_approval" }));
    expect(settleAfterSave(current, sent, response).actions.answer).toBe("denied");
  });

  it("a response that does not name answer leaves it unnamed", () => {
    const sent = draftFromSettings(settings({ ...SIX }));
    const settled = settleAfterSave(sent, sent, sent);
    expect(settled.actions.answer).toBeUndefined();
  });
});
