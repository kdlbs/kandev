import { describe, expect, it } from "vitest";
import * as editorHistory from "./tiptap-editor-history";

describe("rich message history", () => {
  it("provides a structured history-to-editor conversion", () => {
    expect(typeof (editorHistory as Record<string, unknown>).historyEntryToEditorContent).toBe(
      "function",
    );
  });

  it("restores a reference atom only from matching history metadata", () => {
    const reference = {
      version: 1,
      ref: "mention:v1:jira:issue:scope:10001",
      provider: "jira",
      kind: "issue",
      id: "10001",
      key: "ENG-123",
      title: "Fix authentication",
      url: "https://jira.example.test/browse/ENG-123",
      scope: "jira.example.test/site-1",
    };

    const content = editorHistory.historyEntryToEditorContent(
      {
        content: "See [#ENG-123](https://jira.example.test/browse/ENG-123)",
        entityReferences: [reference],
      },
      [],
    );

    expect(content.content?.[0]?.content).toEqual([
      { type: "text", text: "See " },
      { type: "entityReference", attrs: reference },
    ]);
  });
});
