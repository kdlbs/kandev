import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { Message } from "@/lib/types/http";
import { ToolSearchMessage } from "./tool-search-message";

function searchComment(files?: string[], fileCount?: number): Message {
  return {
    id: "m1",
    metadata: {
      status: "complete",
      normalized: {
        code_search: { pattern: "useTranslation", output: { files, file_count: fileCount } },
      },
    },
  } as unknown as Message;
}

// The header label used to build its own "s" in a template literal, which the
// jsx-only lint rule and the inline-plural check both miss. It now selects
// _one/_other, and the English has to stay byte-identical.
describe("ToolSearchMessage", () => {
  it("uses the singular header label for a single hit", () => {
    const html = renderToStaticMarkup(<ToolSearchMessage comment={searchComment(["a.ts"])} />);
    expect(html).toContain("Found 1 file");
    expect(html).not.toContain("Found 1 files");
  });

  it("uses the plural header label for several hits", () => {
    const html = renderToStaticMarkup(
      <ToolSearchMessage comment={searchComment(["a.ts", "b.ts", "c.ts"])} />,
    );
    expect(html).toContain("Found 3 files");
  });

  it("prefers the reported file_count over the returned file list length", () => {
    // The backend truncates `files` but keeps the true total in `file_count`,
    // so the label must plural-select on the count it actually renders.
    const html = renderToStaticMarkup(<ToolSearchMessage comment={searchComment(["a.ts"], 12)} />);
    expect(html).toContain("Found 12 files");
  });

  it("falls back to the searching label before any result arrives", () => {
    const html = renderToStaticMarkup(<ToolSearchMessage comment={searchComment([])} />);
    expect(html).toContain("Searching");
    expect(html).not.toContain("Found");
  });
});
