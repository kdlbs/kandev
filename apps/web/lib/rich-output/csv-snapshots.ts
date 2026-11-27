import { z } from "zod";
import { inlineChartProps, inlineTableProps, type CSVSource, type TableProps } from "./catalog";
import { withinPayloadBound, type CompositionInput, type CompositionElement } from "./validate";
import type { RichOutputPresentation } from "./normalize";

const chartSnapshot = z.strictObject({
  element_id: z.string(),
  labels: inlineChartProps.shape.labels,
  series: inlineChartProps.shape.series,
});
const tableSnapshot = z.strictObject({
  element_id: z.string(),
  columns: inlineTableProps.shape.columns,
  rows: inlineTableProps.shape.rows,
});
const snapshotSchema = z.strictObject({
  version: z.literal(2),
  catalog_version: z.literal(1),
  resolved_charts: z.array(chartSnapshot).min(1).max(4).optional(),
  resolved_tables: z.array(tableSnapshot).min(1).max(4).optional(),
});

function bindChart(element: CompositionElement, snapshot: z.infer<typeof chartSnapshot>) {
  const source = element.props.csv as CSVSource;
  const labels = source.series.map((s) => s.label ?? s.column);
  if (
    labels.length !== snapshot.series.length ||
    snapshot.series.some(
      (s, i) => s.label !== labels[i] || s.values.length !== snapshot.labels.length,
    )
  )
    return null;
  const { csv: _source, ...header } = element.props;
  return { ...element, props: { ...header, labels: snapshot.labels, series: snapshot.series } };
}

function bindTable(element: CompositionElement, snapshot: z.infer<typeof tableSnapshot>) {
  const columns = (element.props as TableProps).columns;
  if (
    columns.length !== snapshot.columns.length ||
    columns.some(
      (c, i) => c.key !== snapshot.columns[i].key || c.label !== snapshot.columns[i].label,
    ) ||
    snapshot.rows.some((row) => row.length !== columns.length)
  )
    return null;
  const { csv: _source, ...header } = element.props;
  return { ...element, props: { ...header, rows: snapshot.rows } };
}

function hasSnapshots(result: unknown): boolean {
  return (
    typeof result === "object" &&
    result !== null &&
    ("resolved_charts" in result || "resolved_tables" in result)
  );
}

function snapshotBindings(
  snapshot: z.infer<typeof snapshotSchema>,
  sources: [string, CompositionElement][],
) {
  const charts = new Map((snapshot.resolved_charts ?? []).map((s) => [s.element_id, s]));
  const tables = new Map((snapshot.resolved_tables ?? []).map((s) => [s.element_id, s]));
  const chartCount = sources.filter(([, e]) => e.type === "Chart").length;
  const tableCount = sources.length - chartCount;
  if (
    charts.size !== chartCount ||
    tables.size !== tableCount ||
    charts.size !== (snapshot.resolved_charts?.length ?? 0) ||
    tables.size !== (snapshot.resolved_tables?.length ?? 0)
  )
    return null;
  return { charts, tables };
}

export function resolveComposition(
  input: CompositionInput,
  result: unknown,
): RichOutputPresentation | null {
  const sources = Object.entries(input.spec.elements).filter(
    ([, e]) => (e.type === "Chart" || e.type === "Table") && "csv" in e.props,
  );
  if (sources.length === 0) {
    if (hasSnapshots(result)) return null;
    return { title: input.title, description: input.description, spec: input.spec };
  }
  if (!withinPayloadBound(result)) return null;
  const parsed = snapshotSchema.safeParse(result);
  if (!parsed.success) return null;
  const bindings = snapshotBindings(parsed.data, sources);
  if (!bindings) return null;
  const { charts, tables } = bindings;
  const elements = { ...input.spec.elements };
  for (const [id, element] of sources) {
    const chart = charts.get(id);
    const table = tables.get(id);
    const resolved =
      element.type === "Chart"
        ? chart && bindChart(element, chart)
        : table && bindTable(element, table);
    if (!resolved) return null;
    elements[id] = resolved;
  }
  return {
    title: input.title,
    description: input.description,
    spec: { root: input.spec.root, elements },
  };
}
