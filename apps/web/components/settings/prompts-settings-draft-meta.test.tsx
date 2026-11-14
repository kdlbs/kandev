import { describe, expect, it } from "vitest";

import type { CustomPrompt } from "@/lib/types/http";
import { getPromptDraftMeta } from "./prompts-settings";

// @covers AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.12
describe("getPromptDraftMeta", () => {
  it("treats an opened create form as a dirty route draft", () => {
    expect(getPromptDraftMeta([], null, true, { name: "", content: "" })).toEqual({
      isDirty: true,
      revision: 'new:{"name":"","content":""}',
    });
  });

  it("tracks only name and content when legacy permission metadata changes", () => {
    const prompt: CustomPrompt = {
      id: "prompt-1",
      name: "Review",
      content: "Review this change",
      builtin: false,
      created_at: "2026-08-20T10:00:00Z",
      updated_at: "2026-08-20T10:00:00Z",
    };
    const formState = { name: prompt.name, content: prompt.content };
    const disabled = getPromptDraftMeta(
      [{ ...prompt, allow_agent_edits: false }],
      prompt.id,
      false,
      formState,
    );
    const enabled = getPromptDraftMeta(
      [{ ...prompt, allow_agent_edits: true }],
      prompt.id,
      false,
      formState,
    );

    expect(disabled.isDirty).toBe(false);
    expect(enabled).toEqual(disabled);
    expect(disabled.revision).toBe('prompt-1:{"name":"Review","content":"Review this change"}');
  });
});
