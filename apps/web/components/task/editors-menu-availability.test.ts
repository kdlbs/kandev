import { describe, expect, it } from "vitest";
import type { EditorOption } from "@/lib/types/http";
import {
  getAvailableTaskTopbarEditors,
  resolveTaskTopbarEditorId,
} from "./editors-menu-availability";

const editor = (overrides: Partial<EditorOption>): EditorOption => ({
  id: "editor",
  type: "editor",
  name: "Editor",
  kind: "custom",
  installed: true,
  enabled: true,
  ...overrides,
});
const EMBEDDED_EDITOR_ID = "internal-vscode";

describe("getAvailableTaskTopbarEditors", () => {
  const editors = [
    editor({ id: EMBEDDED_EDITOR_ID, kind: "internal_vscode", name: "VS Code (Embedded)" }),
    editor({ id: "zed", name: "Zed" }),
    editor({ id: "disabled", enabled: false }),
    editor({ id: "missing", kind: "built_in", installed: false }),
  ];

  it("hides embedded VS Code when the active session does not support it", () => {
    for (const supported of [false, undefined]) {
      expect(getAvailableTaskTopbarEditors(editors, supported).map(({ id }) => id)).toEqual([
        "zed",
      ]);
    }
  });

  it("keeps embedded VS Code when the active session supports it", () => {
    expect(getAvailableTaskTopbarEditors(editors, true).map(({ id }) => id)).toEqual([
      "internal-vscode",
      "zed",
    ]);
  });
});

describe("resolveTaskTopbarEditorId", () => {
  const editors = [editor({ id: "zed" }), editor({ id: "vim" })];

  it("falls back when the saved default is filtered out", () => {
    expect(resolveTaskTopbarEditorId(EMBEDDED_EDITOR_ID, editors)).toBe("zed");
  });

  it("returns no editor when none are available", () => {
    expect(resolveTaskTopbarEditorId(EMBEDDED_EDITOR_ID, [])).toBe("");
  });
});
