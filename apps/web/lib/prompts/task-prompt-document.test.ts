import { describe, expect, it } from "vitest";
import {
  buildTaskPromptDocument,
  collectTaskPromptReferences,
  serializeTaskPromptDocument,
} from "./task-prompt-document";

const PROMPT_NAME = "Daily Summary";
const PROMPT_ALIAS = `@${PROMPT_NAME}`;

describe("task prompt document", () => {
  it("round-trips aliases, whitespace, line breaks, and Unicode text", () => {
    const draft = `😀 Before ${PROMPT_ALIAS}\nAfter  ${PROMPT_ALIAS}  🚀`;
    const document = buildTaskPromptDocument(draft, [PROMPT_NAME]);

    expect(serializeTaskPromptDocument(document)).toBe(draft);
    expect(collectTaskPromptReferences(document)).toEqual([
      { name: PROMPT_NAME, value: PROMPT_ALIAS },
      { name: PROMPT_NAME, value: PROMPT_ALIAS },
    ]);
  });

  it("keeps unknown names, code, and link destinations as plain text", () => {
    const draft =
      `unknown @Missing and \`${PROMPT_ALIAS}\` and ` +
      `[label ${PROMPT_ALIAS}](/docs "title ${PROMPT_ALIAS}") and ${PROMPT_ALIAS}`;
    const document = buildTaskPromptDocument(draft, [PROMPT_NAME]);

    expect(serializeTaskPromptDocument(document)).toBe(draft);
    expect(collectTaskPromptReferences(document)).toEqual([
      { name: PROMPT_NAME, value: PROMPT_ALIAS },
      { name: PROMPT_NAME, value: PROMPT_ALIAS },
    ]);
  });
});
