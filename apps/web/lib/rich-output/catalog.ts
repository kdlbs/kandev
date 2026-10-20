import { defineCatalog, type InferComponentProps } from "@json-render/core";
import { schema } from "@json-render/react/schema";
import { z } from "zod";

function text(minLength: number, maxLength: number) {
  return z
    .string()
    .refine((value) => {
      const length = Array.from(value).length;
      return length >= minLength && length <= maxLength;
    })
    .meta({ minLength, maxLength });
}

const series = z.strictObject({
  label: text(1, 120),
  values: z.array(z.number().finite().nullable()).min(1).max(100),
});
const chartHeader = {
  chart_type: z.enum(["line", "bar"]),
  title: text(1, 120),
  summary: text(1, 500),
};
export const inlineChartProps = z.strictObject({
  ...chartHeader,
  labels: z.array(text(1, 120)).min(1).max(100),
  series: z.array(series).min(1).max(4),
});
export const csvSource = z.strictObject({
  // i18n-exempt: Agent-facing CSV source metadata.
  path: text(1, 1024).describe("One workspace-relative CSV path ending in .csv."),
  repo: text(0, 255).optional(),
  // i18n-exempt: Agent-facing CSV column metadata.
  x_column: text(1, 120).describe("Exact CSV header for x-axis labels."),
  series: z
    .array(
      z.strictObject({
        column: text(1, 120).describe("Exact numeric column header."),
        label: text(1, 120).optional(),
      }),
    )
    .min(1)
    .max(4),
});
export const tableColumns = z
  .array(z.strictObject({ key: text(1, 120), label: text(1, 120) }))
  .min(1)
  .max(8);
export const inlineTableProps = z.strictObject({
  title: text(0, 120).optional(),
  columns: tableColumns,
  rows: z.array(z.array(text(0, 500)).min(1).max(8)).max(100),
});
const tableCSVSource = z.strictObject({ path: text(1, 1024), repo: text(0, 255).optional() });
export const componentProps = {
  Stack: z.strictObject({ direction: z.enum(["vertical", "horizontal"]) }),
  Group: z.strictObject({ title: text(0, 120).optional() }),
  Text: z.strictObject({ text: text(1, 500) }),
  Chart: z.union([inlineChartProps, z.strictObject({ ...chartHeader, csv: csvSource })]),
  Table: z.union([
    inlineTableProps,
    z.strictObject({ title: text(0, 120).optional(), columns: tableColumns, csv: tableCSVSource }),
  ]),
  Callout: z.strictObject({
    kind: z.enum(["info", "success", "warning", "error"]),
    title: text(0, 120).optional(),
    text: text(1, 500),
  }),
  StatusList: z.strictObject({
    title: text(0, 120).optional(),
    items: z
      .array(
        z.strictObject({
          label: text(1, 120),
          status: z.enum(["passed", "failed", "running", "pending", "skipped"]),
          detail: text(0, 500).optional(),
        }),
      )
      .max(20),
  }),
  Metrics: z.strictObject({
    items: z
      .array(
        z.strictObject({
          label: text(1, 120),
          value: text(1, 200),
          detail: text(0, 500).optional(),
        }),
      )
      .min(1)
      .max(6),
  }),
  FilePreview: z.strictObject({
    path: text(1, 1024),
    repo: text(0, 255).optional(),
    title: text(0, 120).optional(),
    caption: text(0, 500).optional(),
  }),
};

// i18n-exempt: English metadata for the agent tool contract, not browser copy.
export const catalog = defineCatalog(schema, {
  components: {
    Stack: {
      props: componentProps.Stack,
      slots: ["default"],
      description: "Ordered vertical or horizontal group. Horizontal groups stack on phones.",
    },
    Group: {
      props: componentProps.Group,
      slots: ["default"],
      description: "Labelled group of native content.",
    },
    Text: { props: componentProps.Text, description: "Plain-text explanation; no markup." },
    Chart: {
      props: componentProps.Chart,
      description:
        "Native line or bar chart with inline data or a workspace CSV source. Label series with units.",
    },
    Table: {
      props: componentProps.Table,
      description:
        "Structured multi-row comparison with labelled columns and literal cells, inline or selected from a workspace CSV. Use Markdown for small textual tables. Phone rows retain every field.",
    },
    Callout: {
      props: componentProps.Callout,
      description:
        "Prominent static observation with info, success, warning or error kind, optional title and literal text. Use for a key finding or issue worth investigating. It is a historical note, not a live alert or tool failure.",
    },
    StatusList: {
      props: componentProps.StatusList,
      description:
        "Ordered verification findings with passed, failed, running, pending or skipped labels. These are static agent-recorded snapshots, never live task status. Use for explaining checks and their evidence.",
    },
    Metrics: {
      props: componentProps.Metrics,
      description: "One to six compact label/value metrics.",
    },
    FilePreview: {
      props: componentProps.FilePreview,
      description: "Workspace file reference. The user explicitly previews or opens it.",
    },
  },
  actions: {},
});

export type ChartProps = z.infer<typeof inlineChartProps>;
export type MetricsProps = InferComponentProps<typeof catalog, "Metrics">;
export type FilePreviewProps = InferComponentProps<typeof catalog, "FilePreview">;
export type ComponentName = keyof typeof componentProps;
export type CSVSource = z.infer<typeof csvSource>;

export type TableProps = z.infer<typeof inlineTableProps>;

export type StatusListProps = InferComponentProps<typeof catalog, "StatusList">;

export type CalloutProps = InferComponentProps<typeof catalog, "Callout">;
