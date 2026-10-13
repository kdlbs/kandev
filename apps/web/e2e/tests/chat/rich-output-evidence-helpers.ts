import fs from "node:fs";
import path from "node:path";
import { seedRichOutputTask } from "./rich-output-helpers";

export const TABLE_FILE = "evidence-services.csv";
export const tableColumns = [
  { key: "service", label: "Service" },
  { key: "cost", label: "Cost (USD)" },
  { key: "p95", label: "p95 (ms)" },
  { key: "uptime", label: "Uptime (%)" },
  { key: "note", label: "Note" },
  { key: "literal", label: "Literal" },
  { key: "formula", label: "Formula" },
  { key: "detail", label: "Detail" },
];
export const tableRows = [
  [
    "API",
    "00420",
    "165",
    "99.98",
    "",
    "<script>literal</script>",
    "=1+2",
    "long-detail-".repeat(18),
  ],
];
export function tableReport() {
  return {
    version: 2,
    catalog_version: 1,
    title: "Evidence Table report",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["csv", "inline", "empty", "file"],
        },
        csv: {
          type: "Table",
          props: { title: "CSV evidence", columns: tableColumns, csv: { path: TABLE_FILE } },
          children: [],
        },
        inline: {
          type: "Table",
          props: { title: "Inline evidence", columns: tableColumns, rows: tableRows },
          children: [],
        },
        empty: {
          type: "Table",
          props: {
            title: "Empty evidence",
            columns: tableColumns,
            csv: { path: "empty-evidence.csv" },
          },
          children: [],
        },
        file: {
          type: "FilePreview",
          props: { path: "rich-output-report.txt", title: "Source notes" },
          children: [],
        },
      },
    },
  };
}
export async function seedEvidenceTask(
  options: Parameters<typeof seedRichOutputTask>[0],
  payload: unknown,
) {
  fs.writeFileSync(
    path.join(options.seedData.repositoryPath, TABLE_FILE),
    [
      tableColumns.map((c) => c.key).join(","),
      ...tableRows.map((row) => row.map((value) => JSON.stringify(value)).join(",")),
    ].join("\n") + "\n",
  );
  fs.writeFileSync(
    path.join(options.seedData.repositoryPath, "empty-evidence.csv"),
    tableColumns.map((c) => c.key).join(",") + "\n",
  );
  return seedRichOutputTask({ ...options, payload });
}

export const statusItems = [
  { label: "Unit tests", status: "passed", detail: "38 checks passed" },
  { label: "Load budget", status: "failed", detail: "p95 over target" },
  { label: "Build snapshot", status: "running", detail: "Recorded state, not live execution" },
  { label: "Dependency audit", status: "pending", detail: "Not checked" },
  { label: "Optional checks", status: "skipped", detail: "long-explanation-".repeat(25) },
];
export const statusLabels = ["Passed", "Failed", "Running", "Pending", "Skipped"];
export function statusReport() {
  return {
    version: 2,
    catalog_version: 1,
    title: "StatusList evidence",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["table", "checks", "empty", "file"],
        },
        table: {
          type: "Table",
          props: { columns: tableColumns.slice(0, 2), rows: [["API", "00420"]] },
          children: [],
        },
        checks: {
          type: "StatusList",
          props: { title: "Verification", items: statusItems },
          children: [],
        },
        empty: {
          type: "StatusList",
          props: { title: "Unrecorded checks", items: [] },
          children: [],
        },
        file: {
          type: "FilePreview",
          props: { path: "rich-output-report.txt", title: "Source notes" },
          children: [],
        },
      },
    },
  };
}

export const calloutLabels = ["Information", "Success", "Warning", "Error"];
export function combinedReport() {
  const kinds = ["info", "success", "warning", "error"];
  return {
    version: 2,
    catalog_version: 1,
    title: "Callout combined evidence",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["table", "checks", "chart", ...kinds.map((kind) => `note_${kind}`), "file"],
        },
        table: {
          type: "Table",
          props: {
            title: "Services",
            columns: tableColumns.slice(0, 3),
            csv: { path: TABLE_FILE },
          },
          children: [],
        },
        checks: {
          type: "StatusList",
          props: { title: "Verification", items: statusItems },
          children: [],
        },
        chart: {
          type: "Chart",
          props: {
            chart_type: "bar",
            title: "Service costs",
            summary: "Monthly service costs and budget in USD",
            labels: ["API", "Payments"],
            series: [
              { label: "Cost (USD)", values: [420, 680] },
              { label: "Budget (USD)", values: [500, 600] },
            ],
          },
          children: [],
        },
        ...Object.fromEntries(
          kinds.map((kind) => [
            `note_${kind}`,
            {
              type: "Callout",
              props: {
                kind,
                title: `Recorded ${kind} finding`,
                text: "<script>literal note</script> " + "long-finding-".repeat(30),
              },
              children: [],
            },
          ]),
        ),
        file: {
          type: "FilePreview",
          props: { path: "rich-output-report.txt", title: "Source notes" },
          children: [],
        },
      },
    },
  };
}
