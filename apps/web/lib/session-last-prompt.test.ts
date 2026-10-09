import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { TASK_DESCRIPTION_SYNTHETIC_ID } from "@/hooks/initial-prompt-preview";
import {
  comparePromptOrder,
  findLastStoredUserPromptIndex,
  isStoredUserPrompt,
  isValidPromptMessage,
  resolveLastPromptMessage,
  type ObservedPrompts,
} from "./session-last-prompt";

const EARLIER = "2026-08-22T00:00:00Z";
const LATER = "2026-08-22T00:00:01Z";
function row(id: string, created_at = EARLIER): Message {
  return {
    id,
    session_id: "s",
    author_type: "user",
    type: "message",
    content: id,
    created_at,
  } as Message;
}

const observed = (id: string, created_at = EARLIER): ObservedPrompts => ({
  ids: { [id]: true },
  newestKey: { id, created_at },
});

describe("last prompt resolution (AC-UI-PINNED-PROMPT-AVAILABILITY-001.5/.6)", () => {
  it("excludes synthetic task descriptions without excluding unparseable stored window prompts", () => {
    const synthetic = row(TASK_DESCRIPTION_SYNTHETIC_ID, "");
    const unparseable = row("unparseable", "not-a-date");
    expect(isStoredUserPrompt(synthetic)).toBe(false);
    expect(isStoredUserPrompt(unparseable)).toBe(true);
    expect(isValidPromptMessage(unparseable)).toBe(false);
    expect(findLastStoredUserPromptIndex([synthetic, unparseable])).toBe(1);
    expect(resolveLastPromptMessage([synthetic, unparseable], [], undefined)).toBe(unparseable);
  });

  it("falls back to the raw loaded window until an observation admits the projection", () => {
    const windowPrompt = row("window");
    const newer = row("projection", LATER);
    expect(resolveLastPromptMessage([windowPrompt], [newer], undefined)).toBe(windowPrompt);
    expect(resolveLastPromptMessage([windowPrompt], [newer], observed("projection", LATER))).toBe(
      newer,
    );
    expect(
      resolveLastPromptMessage([windowPrompt], [row("older", EARLIER)], observed("older")),
    ).toBe(windowPrompt);
    expect(
      resolveLastPromptMessage([row(TASK_DESCRIPTION_SYNTHETIC_ID, "")], [newer], undefined),
    ).toBeNull();
  });

  it("admits a strictly newer cache entry after an observed row leaves the cache", () => {
    const newer = row("newer", LATER);
    expect(resolveLastPromptMessage([row("agent")], [newer], observed("gone"))).toBe(newer);
    expect(resolveLastPromptMessage([], [newer], observed("gone"))).toBe(newer);
  });

  it("keeps the loaded row on an order tie unless the projection version is strictly newer", () => {
    const loaded = { ...row("same"), content: "loaded", updated_at: EARLIER };
    const projected = { ...loaded, content: "projection", updated_at: LATER };
    expect(comparePromptOrder(loaded, projected)).toBe(0);
    expect(
      resolveLastPromptMessage([loaded], [{ ...projected, updated_at: EARLIER }], observed("same")),
    ).toBe(loaded);
    expect(resolveLastPromptMessage([loaded], [projected], observed("same"))).toBe(projected);
  });
});
