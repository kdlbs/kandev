import { describe, expect, it } from "vitest";
import { buildDocumentContentChanges, buildDocumentSaveParams } from "./lsp-document-sync";

describe("buildDocumentContentChanges", () => {
  it.each([1, { change: 1 }])("sends full text for full synchronization (%j)", (sync) => {
    expect(buildDocumentContentChanges({ textDocumentSync: sync }, "before", "after")).toEqual([
      { text: "after" },
    ]);
  });

  it("builds one minimal UTF-16 ranged change for incremental synchronization", () => {
    expect(
      buildDocumentContentChanges(
        { textDocumentSync: { change: 2 } },
        "head\n😀foo\ntail",
        "head\n😀bar\ntail",
      ),
    ).toEqual([
      {
        range: {
          start: { line: 1, character: 2 },
          end: { line: 1, character: 5 },
        },
        text: "bar",
      },
    ]);
  });

  it("does not split a UTF-16 surrogate pair at a change boundary", () => {
    expect(buildDocumentContentChanges({ textDocumentSync: 2 }, "😀 stable", "😁 stable")).toEqual([
      {
        range: {
          start: { line: 0, character: 0 },
          end: { line: 0, character: 2 },
        },
        text: "😁",
      },
    ]);
  });

  it("does not split a CRLF line ending at a change boundary", () => {
    expect(buildDocumentContentChanges({ textDocumentSync: 2 }, "a\r\nb", "a\nb")).toEqual([
      {
        range: {
          start: { line: 0, character: 1 },
          end: { line: 1, character: 0 },
        },
        text: "\n",
      },
    ]);
  });

  it.each([undefined, 0, { change: 0 }, { change: 99 }])(
    "does not synchronize when the server advertises no supported change kind (%j)",
    (sync) => {
      expect(buildDocumentContentChanges({ textDocumentSync: sync }, "before", "after")).toEqual(
        [],
      );
    },
  );
});

describe("buildDocumentSaveParams", () => {
  const uri = "file:///workspace/Main.kt";
  const savedText = "saved text";

  it.each([true, {}, { includeText: false }])(
    "builds a didSave notification without text for save support (%j)",
    (save) => {
      expect(buildDocumentSaveParams({ textDocumentSync: { save } }, uri, savedText)).toEqual({
        textDocument: { uri },
      });
    },
  );

  it("includes the persisted snapshot when the server requests it", () => {
    expect(
      buildDocumentSaveParams(
        { textDocumentSync: { save: { includeText: true } } },
        uri,
        savedText,
      ),
    ).toEqual({ textDocument: { uri }, text: savedText });
  });

  it.each([undefined, 1, { change: 2 }, { save: false }, { save: "invalid" }])(
    "does not notify when save synchronization is unsupported (%j)",
    (sync) => {
      expect(buildDocumentSaveParams({ textDocumentSync: sync }, uri, savedText)).toBeNull();
    },
  );
});
