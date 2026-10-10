import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import { KandevToolMessage } from "./kandev-tool-message";

describe("KandevToolMessage CSV rich output", () => {
  it("renders a CSV chart from the persisted tool-result snapshot", () => {
    const message: Message = {
      id: "msg-1",
      session_id: sessionId("s1"),
      task_id: taskId("t1"),
      author_type: "agent",
      content: "mcp__kandev__show_rich_output_kandev",
      type: "tool_call",
      created_at: "2026-05-21T10:00:00Z",
      metadata: {
        title: "mcp__kandev__show_rich_output_kandev",
        status: "complete",
        normalized: {
          kind: "generic",
          generic: {
            name: "other",
            input: {
              version: 1,
              title: "CSV presentation",
              blocks: [
                {
                  type: "chart",
                  chart_type: "bar",
                  title: "Requests by route",
                  summary: "Request volume from the workspace CSV.",
                  csv: {
                    path: "reports/routes.csv",
                    x_column: "route",
                    series: [{ column: "requests" }],
                  },
                },
              ],
            },
            output: {
              _meta: null,
              content: [
                {
                  type: "text",
                  text: JSON.stringify({
                    version: 1,
                    resolved_charts: [
                      {
                        block_index: 0,
                        labels: ["/api", "/health"],
                        series: [{ label: "requests", values: [2400, 800] }],
                      },
                    ],
                  }),
                },
              ],
              structuredContent: null,
            },
          },
        },
      },
    };

    const html = renderToStaticMarkup(<KandevToolMessage comment={message} />);

    expect(html).toContain("CSV presentation");
    expect(html).toContain('data-testid="rich-output-chart-bar"');
    expect(html).not.toContain("This presentation is unavailable.");
  });
});
