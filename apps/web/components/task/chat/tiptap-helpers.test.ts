import { describe, expect, it, vi } from "vitest";
import type { EditorView } from "@tiptap/pm/view";
import { getMarkdownText, handleEditorPaste, textToEditorContent } from "./tiptap-helpers";
import * as tiptapHelpers from "./tiptap-helpers";

describe("entity reference trigger", () => {
  it("provides a trigger-context guard for # suggestions", () => {
    expect(typeof (tiptapHelpers as Record<string, unknown>).isEntityReferenceTriggerAllowed).toBe(
      "function",
    );
  });

  it("allows # only at a block start or after whitespace outside code", () => {
    const allowed = tiptapHelpers.isEntityReferenceTriggerAllowed as unknown as (args: {
      textBeforeTrigger: string;
      parentType: string;
      hasCodeMark: boolean;
    }) => boolean;

    expect(allowed({ textBeforeTrigger: "", parentType: "paragraph", hasCodeMark: false })).toBe(
      true,
    );
    expect(
      allowed({ textBeforeTrigger: "Discuss ", parentType: "paragraph", hasCodeMark: false }),
    ).toBe(true);
    expect(
      allowed({ textBeforeTrigger: "issue", parentType: "paragraph", hasCodeMark: false }),
    ).toBe(false);
    expect(allowed({ textBeforeTrigger: "", parentType: "codeBlock", hasCodeMark: false })).toBe(
      false,
    );
    expect(allowed({ textBeforeTrigger: "", parentType: "paragraph", hasCodeMark: true })).toBe(
      false,
    );
  });
});

describe("entity reference metadata", () => {
  it("provides structured extraction from editor JSON", () => {
    expect(typeof (tiptapHelpers as Record<string, unknown>).extractEntityReferences).toBe(
      "function",
    );
  });

  it("extracts valid atoms in first-appearance order and deduplicates by ref", () => {
    const reference = {
      version: 1,
      ref: "mention:v1:linear:issue:linear.app%2Forg-1:issue-1",
      provider: "linear",
      kind: "issue",
      id: "issue-1",
      key: "ENG-1",
      title: "Fix login",
      url: "https://linear.app/acme/issue/ENG-1/fix-login",
      scope: "linear.app/org-1",
    };
    const extract = tiptapHelpers.extractEntityReferences as unknown as (doc: {
      content: unknown[];
    }) => (typeof reference)[];

    expect(
      extract({
        content: [
          {
            type: "paragraph",
            content: [
              { type: "entityReference", attrs: reference },
              { type: "entityReference", attrs: reference },
              { type: "entityReference", attrs: { ref: "malformed" } },
            ],
          },
        ],
      }),
    ).toEqual([reference]);
  });
});

describe("getMarkdownText", () => {
  it("serializes entity reference atoms as portable Markdown links", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "Track " },
                {
                  type: "entityReference",
                  attrs: {
                    key: "ENG-123",
                    title: "Fix [auth] flow",
                    url: "https://jira.example.test/browse/ENG-123",
                  },
                },
              ],
            },
          ],
        }),
      }),
    ).toBe("Track [#ENG-123](https://jira.example.test/browse/ENG-123)");
  });

  it("serializes slash command chips as slash command text", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "please run " },
                { type: "slashCommand", attrs: { label: "/slow" } },
                { type: "text", text: " 1s" },
              ],
            },
          ],
        }),
      }),
    ).toBe("please run /slow 1s");
  });

  it("serializes slash command chips from commandName-only attrs", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "please run " },
                { type: "slashCommand", attrs: { commandName: "slow" } },
              ],
            },
          ],
        }),
      }),
    ).toBe("please run /slow");
  });

  it("normalizes slash command chip labels when serializing", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [{ type: "slashCommand", attrs: { label: "slow" } }],
            },
          ],
        }),
      }),
    ).toBe("/slow");
  });
});

describe("textToEditorContent", () => {
  const slowCommand = {
    id: "agent-slow",
    label: "/slow",
    description: "Run a slow response",
    action: "agent" as const,
    agentCommandName: "slow",
  };

  it("restores known slash commands as slash command nodes", () => {
    const content = textToEditorContent("/slow 1s", [slowCommand]);

    expect(content).toEqual({
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "slashCommand",
              attrs: {
                id: "agent-slow",
                label: "/slow",
                commandName: "slow",
                description: "Run a slow response",
              },
            },
            { type: "text", text: " 1s" },
          ],
        },
      ],
    });
  });

  it("does not restore slash command chips inside recalled code fences", () => {
    const content = textToEditorContent("```python\n/slow arg\n```", [slowCommand]);

    expect(JSON.stringify(content)).not.toContain('"slashCommand"');
    expect(content).toEqual({
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "```python" }] },
        { type: "paragraph", content: [{ type: "text", text: "/slow arg" }] },
        { type: "paragraph", content: [{ type: "text", text: "```" }] },
      ],
    });
  });

  it("restores generated reference Markdown only when matching metadata is present", () => {
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
    const restore = textToEditorContent as unknown as (
      text: string,
      commands: readonly (typeof slowCommand)[],
      references: readonly (typeof reference)[],
    ) => ReturnType<typeof textToEditorContent>;

    const content = restore(
      "See [#ENG-123](https://jira.example.test/browse/ENG-123) and [docs](https://example.test)",
      [],
      [reference],
    );

    expect(content.content?.[0]?.content).toEqual([
      { type: "text", text: "See " },
      { type: "entityReference", attrs: reference },
      { type: "text", text: " and [docs](https://example.test)" },
    ]);
  });

  it("keeps generated-looking links literal without structured metadata", () => {
    const content = textToEditorContent(
      "See [#ENG-123](https://jira.example.test/browse/ENG-123)",
      [],
    );

    expect(content.content?.[0]?.content).toEqual([
      {
        type: "text",
        text: "See [#ENG-123](https://jira.example.test/browse/ENG-123)",
      },
    ]);
  });
});

const PNG_MIME_TYPE = "image/png";

describe("handleEditorPaste", () => {
  it("uses clipboard files when item conversion does not provide a file", () => {
    const image = new File(["image"], "copied.png", { type: PNG_MIME_TYPE });
    const onImagePaste = vi.fn();
    const preventDefault = vi.fn();
    const event = {
      clipboardData: {
        files: [image],
        items: [],
        getData: () => "",
      },
      preventDefault,
    } as unknown as ClipboardEvent;

    const handled = handleEditorPaste({} as EditorView, event, {
      current: onImagePaste,
    });

    expect(handled).toBe(true);
    expect(onImagePaste).toHaveBeenCalledWith([image]);
    expect(preventDefault).toHaveBeenCalledOnce();
  });

  it("falls back to a readable file item when the clipboard file list is empty", () => {
    const image = new File(["image"], "copied.png", { type: PNG_MIME_TYPE });
    const onImagePaste = vi.fn();
    const preventDefault = vi.fn();
    const event = {
      clipboardData: {
        files: [],
        items: [{ kind: "file", type: PNG_MIME_TYPE, getAsFile: () => image }],
        getData: () => "",
      },
      preventDefault,
    } as unknown as ClipboardEvent;

    const handled = handleEditorPaste({} as EditorView, event, {
      current: onImagePaste,
    });

    expect(handled).toBe(true);
    expect(onImagePaste).toHaveBeenCalledWith([image]);
    expect(preventDefault).toHaveBeenCalledOnce();
  });

  it("does not duplicate a file mirrored in the clipboard file list and items", () => {
    const image = new File(["image"], "copied.png", { type: PNG_MIME_TYPE });
    const onImagePaste = vi.fn();
    const event = {
      clipboardData: {
        files: [image],
        items: [{ kind: "file", type: PNG_MIME_TYPE, getAsFile: () => image }],
        getData: () => "",
      },
      preventDefault: vi.fn(),
    } as unknown as ClipboardEvent;

    handleEditorPaste({} as EditorView, event, {
      current: onImagePaste,
    });

    expect(onImagePaste).toHaveBeenCalledOnce();
    expect(onImagePaste).toHaveBeenCalledWith([image]);
  });

  it("reports image-only HTML when the browser exposes no readable file", () => {
    const onImagePaste = vi.fn();
    const preventDefault = vi.fn();
    const event = {
      clipboardData: {
        files: [],
        items: [{ kind: "string", type: "text/html" }],
        getData: (type: string) =>
          type === "text/html"
            ? '<meta charset="utf-8"><style>img { max-width: 100%; }</style><img src="https://images.example.test/large-image.png" alt="">'
            : "",
      },
      preventDefault,
    } as unknown as ClipboardEvent;

    const handled = handleEditorPaste({} as EditorView, event, {
      current: onImagePaste,
    });

    expect(handled).toBe(true);
    expect(onImagePaste).toHaveBeenCalledWith([], "unreadable-image");
    expect(preventDefault).toHaveBeenCalledOnce();
  });

  it("leaves rich HTML with visible text to the editor", () => {
    const onImagePaste = vi.fn();
    const preventDefault = vi.fn();
    const event = {
      clipboardData: {
        files: [],
        items: [{ kind: "string", type: "text/html" }],
        getData: (type: string) =>
          type === "text/html"
            ? '<p>Visible caption <img src="https://images.example.test/image.png"></p>'
            : "Visible caption",
      },
      preventDefault,
    } as unknown as ClipboardEvent;

    const handled = handleEditorPaste({} as EditorView, event, {
      current: onImagePaste,
    });

    expect(handled).toBe(false);
    expect(onImagePaste).not.toHaveBeenCalled();
    expect(preventDefault).not.toHaveBeenCalled();
  });
});
