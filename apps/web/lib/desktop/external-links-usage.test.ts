import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

function source(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

describe("programmatic external links", () => {
  it.each([
    ["../../components/settings/pty-terminal-dialog.tsx", "PTY login links"],
    ["../../hooks/use-terminal-link-handler.ts", "terminal links"],
    ["../../components/task/browser-panel.tsx", "browser panel tabs"],
  ])("routes %s through the shared desktop-aware opener", (path) => {
    const contents = source(path);

    expect(contents).toContain("openExternalLink");
    expect(contents).not.toContain("window.open(");
  });

  it.each([
    ["../../hooks/use-open-session-in-editor.ts", "editor custom schemes"],
    ["../../app/office/workspace/settings/export/export-preview.tsx", "downloads"],
  ])("preserves WebView-owned window.open for %s", (path) => {
    expect(source(path)).toContain("window.open(");
  });
});
