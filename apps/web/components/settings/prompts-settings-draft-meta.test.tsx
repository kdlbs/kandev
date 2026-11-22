import { describe, expect, it } from "vitest";

import type { CustomPrompt } from "@/lib/types/http";
import { getPromptDraftMeta } from "./use-prompts-page";

const prompt: CustomPrompt = {
  id: "prompt-1",
  name: "Review",
  content: "Review this change",
  builtin: false,
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};
const empty = { name: "", content: "" };

// @covers AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.12
describe("getPromptDraftMeta", () => {
  it("treats an opened create form as a dirty route draft", () => {
    expect(
      getPromptDraftMeta({
        prompts: [],
        showCreate: true,
        createForm: empty,
        drafts: {},
        pending: {},
      }),
    ).toEqual({ isDirty: true, revision: 'new:{"name":"","content":""}' });
  });

  it("is dirty for every edit, including a removed prompt's, but not a removed prompt's switch", () => {
    const edit = { form: { name: "Review", content: "Changed" }, updatedAt: prompt.updated_at };
    const clean = getPromptDraftMeta({
      prompts: [prompt],
      showCreate: false,
      createForm: empty,
      drafts: {},
      pending: { gone: "local" },
    });
    expect(clean.isDirty).toBe(false);
    const orphaned = getPromptDraftMeta({
      prompts: [prompt],
      showCreate: false,
      createForm: empty,
      drafts: { gone: edit },
      pending: {},
    });
    expect(orphaned.isDirty).toBe(true);
    const edited = getPromptDraftMeta({
      prompts: [prompt],
      showCreate: false,
      createForm: empty,
      drafts: { [prompt.id]: edit },
      pending: {},
    });
    expect(edited.isDirty).toBe(true);
    expect(edited.revision).not.toBe(clean.revision);
  });
});
