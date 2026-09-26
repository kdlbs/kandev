import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

function source(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

describe("programmatic external links", () => {
  it.each([
    ["../../components/settings/pty-terminal-view.tsx", "PTY terminal links"],
    ["../../hooks/use-terminal-link-handler.ts", "terminal links"],
    ["../../components/task/browser-panel.tsx", "browser panel tabs"],
  ])("routes %s through the shared desktop-aware opener", (path) => {
    const contents = source(path);

    expect(contents).toContain("openExternalLink");
    expect(contents).not.toContain("window.open(");
  });

  it("preserves WebView-owned window.open for editor custom schemes", () => {
    expect(source("../../hooks/use-open-session-in-editor.ts")).toContain("window.open(");
  });

  it("uses the browser-owned download flow for selected Office exports", () => {
    const contents = source("../../app/office/workspace/settings/export/export-preview.tsx");
    const downloadHelper = source("../utils/file-download.ts");

    expect(contents).toContain('triggerBlobDownload(blob, "kandev-config-selected.zip")');
    expect(downloadHelper).toContain("anchor.download = fileName");
    expect(downloadHelper).toContain("anchor.click()");
  });
});
