import type { CompositionInput } from "./validate";

// i18n-exempt: Sample agent-authored report data for tool discovery, not host UI copy.
export const richOutputExamples: CompositionInput[] = [
  {
    version: 2,
    catalog_version: 1,
    title: "Service failures",
    spec: {
      root: "failures",
      elements: {
        failures: {
          type: "Chart",
          props: {
            chart_type: "bar",
            title: "Failures by service",
            summary: "Failed requests in the latest interval.",
            labels: ["A", "B"],
            series: [{ label: "Count", values: [42, 27] }],
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Latency trend",
    spec: {
      root: "latency",
      elements: {
        latency: {
          type: "Chart",
          props: {
            chart_type: "line",
            title: "p95 latency",
            summary: "Latency across recorded samples.",
            csv: {
              path: "reports/latency.csv",
              x_column: "recorded_at",
              series: [{ column: "p95_ms", label: "p95 (ms)" }],
            },
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Build health",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "horizontal" },
          children: ["checks", "context"],
        },
        checks: { type: "Group", props: { title: "Checks" }, children: ["counts"] },
        counts: {
          type: "Metrics",
          props: { items: [{ label: "Passed", value: "38" }] },
          children: [],
        },
        context: { type: "Text", props: { text: "All focused checks passed." }, children: [] },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Service comparison",
    spec: {
      root: "services",
      elements: {
        services: {
          type: "Table",
          props: {
            columns: [
              { key: "service", label: "Service" },
              { key: "cost", label: "Monthly cost (USD)" },
            ],
            rows: [
              ["API", "420"],
              ["Payments", "680"],
            ],
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "CSV service comparison",
    spec: {
      root: "services",
      elements: {
        services: {
          type: "Table",
          props: {
            columns: [
              { key: "service", label: "Service" },
              { key: "cost", label: "Monthly cost (USD)" },
            ],
            csv: { path: "reports/services.csv" },
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Verification findings",
    spec: {
      root: "checks",
      elements: {
        checks: {
          type: "StatusList",
          props: {
            title: "Verification",
            items: [
              { label: "Unit tests", status: "passed", detail: "38 checks passed." },
              { label: "Load budget", status: "failed", detail: "p95 exceeds target." },
              { label: "Dependency audit", status: "pending" },
            ],
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Main finding",
    spec: {
      root: "finding",
      elements: {
        finding: {
          type: "Callout",
          props: {
            kind: "warning",
            title: "Investigate Payments",
            text: "Payments has the highest monthly cost and p95 latency in this sample.",
          },
          children: [],
        },
      },
    },
  },
  {
    version: 2,
    catalog_version: 1,
    title: "Source context",
    spec: {
      root: "source",
      elements: {
        source: { type: "FilePreview", props: { path: "reports/notes.md" }, children: [] },
      },
    },
  },
];
