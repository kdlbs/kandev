import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { KandevToolMessage } from "./kandev-tool-message";
import { richOutputExamples } from "@/lib/rich-output/examples";
import { kandevToolCall } from "./kandev-tool-message.test-helpers";
import { TooltipProvider } from "@kandev/ui/tooltip";
const UNAVAILABLE = "This presentation is unavailable.";
const RICH_OUTPUT_TEST_ID = 'data-testid="rich-output"';
const COPILOT_TOOL_TITLE = "kandev-show_rich_output_kandev";
const COPILOT_CALLOUT_ARGS = {
  version: 2,
  catalog_version: 1,
  title: "Synthetic report",
  spec: {
    root: "report",
    elements: {
      report: {
        type: "Callout",
        props: {
          kind: "info",
          title: "Synthetic report",
          text: "Synthetic check completed.",
        },
        children: [],
      },
    },
  },
};

function copilotToolCall(status: "pending" | "running" | "error" | "complete" = "complete") {
  return kandevToolCall({
    toolName: COPILOT_TOOL_TITLE,
    input: { kind: "other", raw_input: COPILOT_CALLOUT_ARGS },
    output: { content: "Rich output accepted." },
    status,
  });
}

describe("KandevToolMessage rich output", () => {
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  it.each(["codex", "cursor"])("renders composed output in the %s envelope", (provider) => {
    const args = richOutputExamples[2];
    const raw_input =
      provider === "codex"
        ? { server: "kandev", tool: "show_rich_output_kandev", arguments: args }
        : { providerIdentifier: "kandev", toolName: "show_rich_output_kandev", args };
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName:
            provider === "codex"
              ? "mcp.kandev.show_rich_output_kandev"
              : "kandev: show_rich_output_kandev",
          input: { raw_input },
          output: { success: true },
        })}
      />,
    );
    expect(html).toContain('data-testid="rich-output-group"');
    expect(html).toContain("All focused checks passed.");
    expect(html).not.toContain(UNAVAILABLE);
  });

  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.5, AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  it.each(["structured", "text"])("replays the element-addressed CSV %s result", (format) => {
    const snapshot = {
      version: 2,
      catalog_version: 1,
      resolved_charts: [
        {
          element_id: "latency",
          labels: ["Accepted sample"],
          series: [{ label: "p95 (ms)", values: [29.4] }],
        },
      ],
    };
    const output =
      format === "structured"
        ? { structuredContent: snapshot, content: [] }
        : { content: [{ type: "text", text: JSON.stringify(snapshot) }] };
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp__kandev__show_rich_output_kandev",
          input: richOutputExamples[1],
          output,
        })}
      />,
    );
    expect(html).toContain("Latency across recorded samples.");
    expect(html).toContain('data-testid="rich-output-chart-line"');
    expect(html).not.toContain(UNAVAILABLE);
  });

  it("unwraps the real Codex ACP MCP arguments before rendering rich output", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp.kandev.show_rich_output_kandev",
          input: {
            raw_input: {
              server: "kandev",
              tool: "show_rich_output_kandev",
              arguments: {
                version: 1,
                title: "Nested presentation",
                blocks: [{ type: "metrics", items: [{ label: "Passed", value: "38" }] }],
              },
            },
          },
        })}
      />,
    );

    expect(html).toContain("Nested presentation");
    expect(html).not.toContain(UNAVAILABLE);
  });

  it("renders rich output from the persisted Cursor ACP MCP envelope", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "kandev: show_rich_output_kandev",
          input: {
            kind: "other",
            raw_input: {
              providerIdentifier: "kandev",
              toolName: "show_rich_output_kandev",
              args: {
                version: 1,
                title: "Build health",
                blocks: [{ type: "metrics", items: [{ label: "Passed", value: "38" }] }],
              },
            },
          },
          output: { success: true },
        })}
      />,
    );

    expect(html).toContain(RICH_OUTPUT_TEST_ID);
    expect(html).toContain("Build health");
    expect(html).not.toContain(UNAVAILABLE);
  });
});

describe("Copilot rich output compatibility", () => {
  it("renders the saved Callout without rewriting its metadata", () => {
    const html = renderToStaticMarkup(<KandevToolMessage comment={copilotToolCall()} />);

    expect(html).toContain(RICH_OUTPUT_TEST_ID);
    expect(html).toContain("Synthetic report");
    expect(html).toContain("Synthetic check completed.");
    expect(html).not.toContain(UNAVAILABLE);
  });

  it.each(["pending", "error"] as const)("keeps the existing %s activity surface", (status) => {
    const copilot = renderToStaticMarkup(
      <TooltipProvider>
        <KandevToolMessage comment={copilotToolCall(status)} />
      </TooltipProvider>,
    );
    const canonical = renderToStaticMarkup(
      <TooltipProvider>
        <KandevToolMessage
          comment={kandevToolCall({
            toolName: "mcp__kandev__show_rich_output_kandev",
            input: { kind: "other", raw_input: COPILOT_CALLOUT_ARGS },
            output: { content: "Rich output accepted." },
            status,
          })}
        />
      </TooltipProvider>,
    );

    expect(copilot).toBe(canonical);
    expect(copilot).not.toContain(RICH_OUTPUT_TEST_ID);
  });
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.3, AC-AGENTS-AGENT-RICH-OUTPUT-003.10
it.each(["structured", "text"])(
  "replays the Table CSV snapshot through the %s tool result",
  (format) => {
    const args = richOutputExamples[4];
    const columns = args.spec.elements.services.props.columns;
    const snapshot = {
      version: 2,
      catalog_version: 1,
      resolved_tables: [{ element_id: "services", columns, rows: [["API", "00420"]] }],
    };
    const output =
      format === "structured"
        ? { structuredContent: snapshot }
        : { content: [{ type: "text", text: JSON.stringify(snapshot) }] };
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp__kandev__show_rich_output_kandev",
          input: args,
          output,
        })}
      />,
    );
    expect(html).toContain('data-testid="rich-output-table"');
    expect(html).toContain("00420");
    expect(html).not.toContain(UNAVAILABLE);
    const wrong = {
      ...snapshot,
      resolved_tables: [{ ...snapshot.resolved_tables[0], element_id: "other" }],
    };
    const invalid = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp__kandev__show_rich_output_kandev",
          input: args,
          output: { structuredContent: wrong },
        })}
      />,
    );
    expect(invalid).toContain(UNAVAILABLE);
    expect(invalid).not.toContain("00420");
  },
);

function evidenceEnvelope() {
  const table = richOutputExamples[4].spec.elements.services;
  const chart = richOutputExamples[1].spec.elements.latency;
  const args = {
    version: 2,
    catalog_version: 1,
    title: "Combined evidence",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["services", "checks", "finding", "latency"],
        },
        services: table,
        checks: richOutputExamples[5].spec.elements.checks,
        finding: richOutputExamples[6].spec.elements.finding,
        latency: chart,
      },
    },
  };
  const snapshot = {
    version: 2,
    catalog_version: 1,
    resolved_tables: [
      { element_id: "services", columns: table.props.columns, rows: [["API", "00420"]] },
    ],
    resolved_charts: [
      {
        element_id: "latency",
        labels: ["Sample"],
        series: [{ label: "p95 (ms)", values: [29.4] }],
      },
    ],
  };
  return { args, snapshot };
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.1, AC-AGENTS-AGENT-RICH-OUTPUT-003.8, AC-AGENTS-AGENT-RICH-OUTPUT-003.10
it.each(["structured", "text"])(
  "renders mixed Table/StatusList/Callout from the full %s envelope",
  (format) => {
    const { args, snapshot } = evidenceEnvelope();
    const output =
      format === "structured"
        ? { structuredContent: snapshot }
        : { content: [{ type: "text", text: JSON.stringify(snapshot) }] };
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp.kandev.show_rich_output_kandev",
          input: {
            raw_input: { server: "kandev", tool: "show_rich_output_kandev", arguments: args },
          },
          output,
        })}
      />,
    );
    expect(html).toContain('data-testid="rich-output-table"');
    expect(html).toContain('data-testid="rich-output-status-list"');
    expect(html).toContain('role="note"');
    expect(html).toContain("00420");
    expect(html).not.toContain(UNAVAILABLE);
  },
);

it("rejects an incomplete mixed snapshot and an unrelated tool identity", () => {
  const { args, snapshot } = evidenceEnvelope();
  const html = renderToStaticMarkup(
    <KandevToolMessage
      comment={kandevToolCall({
        toolName: "mcp__kandev__show_rich_output_kandev",
        input: args,
        output: { structuredContent: { ...snapshot, resolved_tables: [] } },
      })}
    />,
  );
  expect(html).toContain(UNAVAILABLE);
  expect(html).not.toContain("00420");
  const unrelated = renderToStaticMarkup(
    <KandevToolMessage
      comment={kandevToolCall({
        toolName: "mcp.other.show_rich_output_kandev",
        input: args,
        output: { structuredContent: snapshot },
      })}
    />,
  );
  expect(unrelated).not.toContain(RICH_OUTPUT_TEST_ID);
});
