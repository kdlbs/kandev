import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { AskUserQuestionRenderer } from "./ask-user-question-renderer";

describe("AskUserQuestionRenderer", () => {
  it("renders lightweight Markdown in every expanded clarification field", () => {
    const html = renderToStaticMarkup(
      <AskUserQuestionRenderer
        status="running"
        args={{
          context: "Keep `context` literal",
          questions: [
            {
              id: "q1",
              title: "Choose `storage`",
              prompt: "Pick **one**:\n\n1. Fast\n2. Durable",
              options: [
                { label: "`Postgres`", description: "For **production**" },
                { label: "SQLite", description: "For *local* work" },
              ],
            },
          ],
        }}
        result={{ responses: { q1: { question_id: "q1", selected: "`Postgres`" } } }}
      />,
    );

    expect(html).toContain("<code");
    expect(html).toContain(">storage</code>");
    expect(html).toContain("<strong>one</strong>");
    expect(html).toContain("<ol");
    expect(html).toContain(">Postgres</code>");
    expect(html).toContain("<strong>production</strong>");
    expect(html).toContain("<em>local</em>");
    expect(html).toContain("Keep `context` literal");
    expect(html).not.toContain(">context</code>");
    expect(html).not.toContain('title="For **production**"');
  });

  it("keeps summary links passive while expanded prompt links stay interactive", () => {
    const html = renderToStaticMarkup(
      <AskUserQuestionRenderer
        status="running"
        args={{
          questions: [
            {
              id: "q1",
              prompt: "Read the [storage guide](https://example.com/storage).",
              options: [],
            },
          ],
        }}
        result={{}}
      />,
    );
    const root = document.createElement("div");
    root.innerHTML = html;
    const header = root.querySelector(".cursor-pointer");
    const expandedBody = root.querySelector(".mt-2.ml-7");

    expect(header?.querySelector('a[href="https://example.com/storage"]')).toBeNull();
    expect(expandedBody?.querySelectorAll('a[href="https://example.com/storage"]')).toHaveLength(1);
  });
});
