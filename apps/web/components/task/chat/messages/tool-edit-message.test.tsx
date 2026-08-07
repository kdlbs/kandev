import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { Message } from "@/lib/types/http";
import { ToolEditMessage } from "./tool-edit-message";

function writeComment(content: string, body: string): Message {
  return {
    id: "m1",
    content,
    metadata: {
      status: "complete",
      normalized: {
        modify_file: {
          file_path: "apps/web/lib/utils.ts",
          mutations: [{ type: "create", content: body, start_line: 1 }],
        },
      },
    },
  } as unknown as Message;
}

// The write-card header used to append "(N line(s))" with a template-literal
// plural. It now selects _one/_other, with the path kept out of the plural
// decision and passed through as an interpolated value.
describe("ToolEditMessage write summary", () => {
  it("uses the singular line suffix for a one-line write", () => {
    const html = renderToStaticMarkup(
      <ToolEditMessage comment={writeComment("Write utils.ts", "only line")} />,
    );
    expect(html).toContain("Write utils.ts (1 line)");
    expect(html).not.toContain("(1 lines)");
  });

  it("uses the plural line suffix for a multi-line write", () => {
    const html = renderToStaticMarkup(
      <ToolEditMessage comment={writeComment("Write utils.ts", "a\nb\nc")} />,
    );
    expect(html).toContain("Write utils.ts (3 lines)");
  });

  it("leaves the summary untouched for an edit that is not a write", () => {
    const comment = {
      id: "m1",
      content: "Edit utils.ts",
      metadata: {
        status: "complete",
        normalized: {
          modify_file: {
            file_path: "apps/web/lib/utils.ts",
            mutations: [{ type: "edit", old_string: "a", new_string: "b", start_line: 1 }],
          },
        },
      },
    } as unknown as Message;
    const html = renderToStaticMarkup(<ToolEditMessage comment={comment} />);
    expect(html).toContain("Edit utils.ts");
    expect(html).not.toContain("line)");
  });
});
