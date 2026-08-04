import { describe, expect, it } from "vitest";
import { resolveActiveLspStatusItem } from "./app-status-items";

describe("resolveActiveLspStatusItem", () => {
  it("selects only the active supported Monaco file", () => {
    expect(
      resolveActiveLspStatusItem({
        placement: "status_bar",
        activeSessionId: "session-1",
        activeFilePath: "app/src/main/kotlin/Main.kt",
        editorProvider: "monaco",
        hasMountedMonacoEditor: true,
      }),
    ).toEqual({
      sessionId: "session-1",
      monacoLanguage: "kotlin",
    });
  });

  it("hides for toolbar placement, missing active files, and unsupported files", () => {
    expect(
      resolveActiveLspStatusItem({
        placement: "toolbar",
        activeSessionId: "session-1",
        activeFilePath: "src/Main.kt",
        editorProvider: "monaco",
        hasMountedMonacoEditor: true,
      }),
    ).toBeNull();
    expect(
      resolveActiveLspStatusItem({
        placement: "status_bar",
        activeSessionId: "session-1",
        activeFilePath: null,
        editorProvider: "monaco",
        hasMountedMonacoEditor: true,
      }),
    ).toBeNull();
    expect(
      resolveActiveLspStatusItem({
        placement: "status_bar",
        activeSessionId: "session-1",
        activeFilePath: "README.md",
        editorProvider: "monaco",
        hasMountedMonacoEditor: true,
      }),
    ).toBeNull();
  });

  it("hides for a supported file rendered by CodeMirror", () => {
    expect(
      resolveActiveLspStatusItem({
        placement: "status_bar",
        activeSessionId: "session-1",
        activeFilePath: "src/Main.kt",
        editorProvider: "codemirror",
        hasMountedMonacoEditor: false,
      }),
    ).toBeNull();
  });

  it("hides until the active file panel has mounted Monaco", () => {
    expect(
      resolveActiveLspStatusItem({
        placement: "status_bar",
        activeSessionId: "session-1",
        activeFilePath: "src/Main.kt",
        editorProvider: "monaco",
        hasMountedMonacoEditor: false,
      }),
    ).toBeNull();
  });
});
