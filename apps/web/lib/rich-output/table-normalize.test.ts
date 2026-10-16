import { describe, expect, it } from "vitest";
import { normalizeRichOutput } from "./normalize";

const columns = [
  { key: "service", label: "Service" },
  { key: "cost", label: "Cost (USD)" },
];
const input = {
  version: 2,
  catalog_version: 1,
  title: "Services",
  spec: {
    root: "services",
    elements: {
      services: { type: "Table", props: { columns, csv: { path: "services.csv" } }, children: [] },
    },
  },
};
const snapshot = {
  version: 2,
  catalog_version: 1,
  resolved_tables: [
    {
      element_id: "services",
      columns,
      rows: [
        ["API", "0420"],
        ["Payments", ""],
      ],
    },
  ],
};
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.3, AC-AGENTS-AGENT-RICH-OUTPUT-003.10
it("binds selected literal table cells to their exact element without changing input", () => {
  const before = JSON.stringify(input);
  expect(normalizeRichOutput(input, snapshot)?.spec.elements.services.props).toEqual({
    columns,
    rows: [
      ["API", "0420"],
      ["Payments", ""],
    ],
  });
  expect(JSON.stringify(input)).toBe(before);
  expect(normalizeRichOutput(input)).toBeNull();
});
describe("table snapshot binding", () => {
  it.each([
    { ...snapshot, version: 1 },
    { ...snapshot, catalog_version: 2 },
    { ...snapshot, resolved_tables: [] },
    { ...snapshot, resolved_tables: [...snapshot.resolved_tables, ...snapshot.resolved_tables] },
    { ...snapshot, resolved_tables: [{ ...snapshot.resolved_tables[0], element_id: "other" }] },
    {
      ...snapshot,
      resolved_tables: [{ ...snapshot.resolved_tables[0], columns: [...columns].reverse() }],
    },
    {
      ...snapshot,
      resolved_tables: [
        {
          ...snapshot.resolved_tables[0],
          columns: [{ ...columns[0], label: "Wrong" }, columns[1]],
        },
      ],
    },
    { ...snapshot, resolved_tables: [{ ...snapshot.resolved_tables[0], rows: [["API"]] }] },
    { ...snapshot, resolved_tables: [{ ...snapshot.resolved_tables[0], rows: [["API", 420]] }] },
    {
      ...snapshot,
      resolved_charts: [
        { element_id: "services", labels: ["API"], series: [{ label: "Cost", values: [420] }] },
      ],
    },
  ])("rejects incomplete, extra or mismatched table snapshots: %j", (result) => {
    expect(normalizeRichOutput(input, result)).toBeNull();
  });
  it("rejects unsolicited table snapshots on a purely inline presentation", () => {
    const inline = structuredClone(input);
    Object.assign(inline.spec.elements.services, { props: { columns, rows: [["API", "420"]] } });
    expect(normalizeRichOutput(inline)).not.toBeNull();
    expect(normalizeRichOutput(inline, snapshot)).toBeNull();
  });
});

it("rejects a table snapshot over the combined 64 KiB envelope bound", () => {
  const rows = Array.from({ length: 100 }, () => ["x".repeat(400), "y".repeat(400)]);
  expect(
    normalizeRichOutput(input, {
      ...snapshot,
      resolved_tables: [{ ...snapshot.resolved_tables[0], rows }],
    }),
  ).toBeNull();
});

it("binds mixed chart and table snapshots by type and identity", () => {
  const mixed = structuredClone(input);
  Object.assign(mixed.spec, {
    root: "report",
    elements: {
      ...mixed.spec.elements,
      report: { type: "Stack", props: { direction: "vertical" }, children: ["services", "costs"] },
      costs: {
        type: "Chart",
        props: {
          chart_type: "bar",
          title: "Costs",
          summary: "Costs in USD",
          csv: {
            path: "services.csv",
            x_column: "service",
            series: [{ column: "cost", label: "USD" }],
          },
        },
        children: [],
      },
    },
  });
  const result = {
    ...snapshot,
    resolved_charts: [
      { element_id: "costs", labels: ["API"], series: [{ label: "USD", values: [420] }] },
    ],
  };
  const presentation = normalizeRichOutput(mixed, result);
  expect(presentation?.spec.elements.costs.props).toMatchObject({
    labels: ["API"],
    series: [{ label: "USD", values: [420] }],
  });
  expect(presentation?.spec.elements.services.props).toMatchObject({
    rows: snapshot.resolved_tables[0].rows,
  });
  expect(
    normalizeRichOutput(mixed, {
      ...result,
      resolved_charts: [{ ...result.resolved_charts[0], element_id: "services" }],
    }),
  ).toBeNull();
});
