import { describe, expect, it } from "vitest";
import { richOutputExamples } from "./examples";
import { validateComposition } from "./validate";
import { normalizeRichOutput as parseRichOutput } from "./normalize";

function composedInput() {
  return {
    version: 2,
    catalog_version: 1,
    title: "Verification",
    spec: {
      root: "report",
      elements: {
        report: { type: "Stack", props: { direction: "horizontal" }, children: ["note", "counts"] },
        note: { type: "Text", props: { text: "Focused checks passed" }, children: [] },
        counts: {
          type: "Metrics",
          props: { items: [{ label: "Passed", value: "38" }] },
          children: [],
        },
      },
    },
  };
}

describe("catalog composition", () => {
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.1
  it("accepts an agent-authored hierarchy rather than only a block list", () => {
    expect(parseRichOutput(composedInput())).not.toBeNull();
  });
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  it.each(["on", "visible", "repeat", "watch", "slots"])("rejects undeclared %s fields", (key) => {
    const input = composedInput();
    Object.assign(input.spec.elements.note, { [key]: {} });
    expect(parseRichOutput(input)).toBeNull();
  });
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.3
  it("rejects cycles and multiple parents", () => {
    const input = composedInput();
    input.spec.elements.report.children.push("report");
    expect(parseRichOutput(input)).toBeNull();
    input.spec.elements.report.children = ["note", "note"];
    expect(parseRichOutput(input)).toBeNull();
  });
});

const csvInput = {
  version: 2,
  catalog_version: 1,
  title: "CSV report",
  spec: {
    root: "latency",
    elements: {
      latency: {
        type: "Chart",
        props: {
          chart_type: "line",
          title: "Latency",
          summary: "Accepted samples",
          csv: { path: "reports/latency.csv", x_column: "time", series: [{ column: "p95_ms" }] },
        },
        children: [],
      },
    },
  },
};
const csvResult = {
  version: 2,
  catalog_version: 1,
  resolved_charts: [
    { element_id: "latency", labels: ["a"], series: [{ label: "p95_ms", values: [29.4] }] },
  ],
};
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.5, AC-AGENTS-AGENT-RICH-OUTPUT-002.6
it("replays the exact CSV snapshot by element identity without modifying stored arguments", () => {
  const before = JSON.stringify(csvInput);
  const output = parseRichOutput(csvInput, csvResult);
  expect(output?.spec.elements.latency.props).toEqual({
    chart_type: "line",
    title: "Latency",
    summary: "Accepted samples",
    labels: ["a"],
    series: [{ label: "p95_ms", values: [29.4] }],
  });
  expect(JSON.stringify(csvInput)).toBe(before);
  expect(parseRichOutput(csvInput)).toBeNull();
});
it.each([
  { ...csvResult, version: 1 },
  { ...csvResult, catalog_version: 2 },
  { ...csvResult, resolved_charts: [] },
  { ...csvResult, resolved_charts: [csvResult.resolved_charts[0], csvResult.resolved_charts[0]] },
  { ...csvResult, resolved_charts: [{ ...csvResult.resolved_charts[0], element_id: "other" }] },
  {
    ...csvResult,
    resolved_charts: [
      { ...csvResult.resolved_charts[0], series: [{ label: "wrong", values: [29.4] }] },
    ],
  },
])("rejects mismatched CSV result identity or version: %j", (result) => {
  expect(parseRichOutput(csvInput, result)).toBeNull();
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.9
it.each(richOutputExamples)("validates the discovery example: $title", (example) => {
  expect(validateComposition(example)).not.toBeNull();
});
