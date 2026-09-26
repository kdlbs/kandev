import type {
  RichOutput,
  RichOutputBlock,
  RichOutputChartBlock,
  RichOutputChartSeries,
  RichOutputCSVChartBlock,
  RichOutputCSVSeries,
  RichOutputCSVSource,
  RichOutputFileBlock,
  RichOutputInput,
  RichOutputInputBlock,
  RichOutputMetric,
  RichOutputMetricsBlock,
} from "./types";

const MAX_PAYLOAD_BYTES = 64 * 1024;

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, allowed: readonly string[]): boolean {
  return Object.keys(value).every((key) => allowed.includes(key));
}

function isBoundedString(value: unknown, min: number, max: number): value is string {
  if (typeof value !== "string") return false;
  const length = Array.from(value).length;
  return length >= min && length <= max;
}

function optionalBoundedString(
  value: Record<string, unknown>,
  key: string,
  max: number,
): string | null | undefined {
  if (!Object.hasOwn(value, key)) return undefined;
  return isBoundedString(value[key], 0, max) ? value[key] : null;
}

function isWorkspaceRelativePath(value: string): boolean {
  if (value.trim() === "" || value.includes("\0")) return false;
  const normalized = value.replaceAll("\\", "/");
  const lower = normalized.toLowerCase();
  if (normalized.startsWith("/") || lower.includes("://") || lower.startsWith("data:")) {
    return false;
  }
  if (/^[a-z]:/i.test(normalized)) return false;
  if (normalized.split("/").includes("..")) return false;
  return normalized.split("/").some((segment) => segment !== "" && segment !== ".");
}

function parseFileBlock(value: Record<string, unknown>): RichOutputFileBlock | null {
  const allowed = ["type", "path", "repo", "title", "caption", "mime_type"];
  if (!hasOnlyKeys(value, allowed) || !isBoundedString(value.path, 1, 1024)) return null;
  if (!isWorkspaceRelativePath(value.path)) return null;
  const repo = optionalBoundedString(value, "repo", 255);
  const title = optionalBoundedString(value, "title", 120);
  const caption = optionalBoundedString(value, "caption", 500);
  const mimeType = optionalBoundedString(value, "mime_type", 128);
  if (repo === null || title === null || caption === null || mimeType === null) return null;
  return {
    type: "file",
    path: value.path,
    ...(repo !== undefined && { repo }),
    ...(title !== undefined && { title }),
    ...(caption !== undefined && { caption }),
    ...(mimeType !== undefined && { mime_type: mimeType }),
  };
}

function parseChartSeries(value: unknown, labelCount: number): RichOutputChartSeries | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ["label", "values"])) return null;
  if (!isBoundedString(value.label, 1, 120) || !Array.isArray(value.values)) return null;
  if (value.values.length < 1 || value.values.length > 100 || value.values.length !== labelCount) {
    return null;
  }
  const values: Array<number | null> = [];
  for (const item of value.values) {
    if (item === null) values.push(null);
    else if (typeof item === "number" && Number.isFinite(item)) values.push(item);
    else return null;
  }
  return { label: value.label, values };
}

function hasValidChartHeader(value: Record<string, unknown>): boolean {
  if (value.chart_type !== "line" && value.chart_type !== "bar") return false;
  return isBoundedString(value.title, 1, 120) && isBoundedString(value.summary, 1, 500);
}

function parseInlineChartBlock(value: Record<string, unknown>): RichOutputChartBlock | null {
  if (!Array.isArray(value.labels) || value.labels.length < 1 || value.labels.length > 100) {
    return null;
  }
  const labels = value.labels;
  if (!labels.every((label) => isBoundedString(label, 1, 120))) return null;
  if (!Array.isArray(value.series) || value.series.length < 1 || value.series.length > 4) {
    return null;
  }
  const series = value.series.map((item) => parseChartSeries(item, labels.length));
  if (series.some((item) => item === null)) return null;
  return {
    type: "chart",
    chart_type: value.chart_type as "line" | "bar",
    title: value.title as string,
    summary: value.summary as string,
    labels,
    series: series as RichOutputChartSeries[],
  };
}

function parseCSVSeries(value: unknown): RichOutputCSVSeries | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ["column", "label"])) return null;
  if (!isBoundedString(value.column, 1, 120)) return null;
  const label = optionalBoundedString(value, "label", 120);
  if (label === null || label === "") return null;
  return { column: value.column, ...(label !== undefined && { label }) };
}

function parseCSVSource(value: unknown): RichOutputCSVSource | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ["path", "repo", "x_column", "series"])) {
    return null;
  }
  if (!isBoundedString(value.path, 1, 1024) || !isWorkspaceRelativePath(value.path)) return null;
  if (!value.path.toLowerCase().endsWith(".csv")) return null;
  if (!isBoundedString(value.x_column, 1, 120) || !Array.isArray(value.series)) return null;
  if (value.series.length < 1 || value.series.length > 4) return null;
  const repo = optionalBoundedString(value, "repo", 255);
  const series = value.series.map(parseCSVSeries);
  if (repo === null || series.some((item) => item === null)) return null;
  return {
    path: value.path,
    ...(repo !== undefined && { repo }),
    x_column: value.x_column,
    series: series as RichOutputCSVSeries[],
  };
}

function parseCSVChartBlock(value: Record<string, unknown>): RichOutputCSVChartBlock | null {
  const csv = parseCSVSource(value.csv);
  if (!csv) return null;
  return {
    type: "chart",
    chart_type: value.chart_type as "line" | "bar",
    title: value.title as string,
    summary: value.summary as string,
    csv,
  };
}

function parseChartBlock(
  value: Record<string, unknown>,
): RichOutputChartBlock | RichOutputCSVChartBlock | null {
  const allowed = ["type", "chart_type", "title", "summary", "labels", "series", "csv"];
  if (!hasOnlyKeys(value, allowed) || !hasValidChartHeader(value)) return null;
  const hasInline = Object.hasOwn(value, "labels") || Object.hasOwn(value, "series");
  const hasCSV = Object.hasOwn(value, "csv");
  if (hasInline === hasCSV) return null;
  return hasCSV ? parseCSVChartBlock(value) : parseInlineChartBlock(value);
}

function parseMetric(value: unknown): RichOutputMetric | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ["label", "value", "detail"])) return null;
  if (!isBoundedString(value.label, 1, 120) || !isBoundedString(value.value, 1, 200)) return null;
  const detail = optionalBoundedString(value, "detail", 500);
  if (detail === null) return null;
  return { label: value.label, value: value.value, ...(detail !== undefined && { detail }) };
}

function parseMetricsBlock(value: Record<string, unknown>): RichOutputMetricsBlock | null {
  if (!hasOnlyKeys(value, ["type", "items"]) || !Array.isArray(value.items)) return null;
  if (value.items.length < 1 || value.items.length > 6) return null;
  const items = value.items.map(parseMetric);
  if (items.some((item) => item === null)) return null;
  return { type: "metrics", items: items as RichOutputMetric[] };
}

function parseBlock(value: unknown): RichOutputInputBlock | null {
  if (!isRecord(value)) return null;
  if (value.type === "file") return parseFileBlock(value);
  if (value.type === "chart") return parseChartBlock(value);
  if (value.type === "metrics") return parseMetricsBlock(value);
  return null;
}

function serializedByteLength(value: unknown): number | null {
  try {
    const serialized = JSON.stringify(value);
    return serialized === undefined ? null : new TextEncoder().encode(serialized).byteLength;
  } catch {
    return null;
  }
}

function parseRichOutputInput(value: unknown): RichOutputInput | null {
  const bytes = serializedByteLength(value);
  if (bytes === null || bytes > MAX_PAYLOAD_BYTES || !isRecord(value)) return null;
  if (!hasOnlyKeys(value, ["version", "title", "description", "blocks"])) return null;
  if (value.version !== 1 || !isBoundedString(value.title, 1, 120)) return null;
  const description = optionalBoundedString(value, "description", 500);
  if (description === null || !Array.isArray(value.blocks)) return null;
  if (value.blocks.length < 1 || value.blocks.length > 4) return null;
  const blocks = value.blocks.map(parseBlock);
  if (blocks.some((block) => block === null)) return null;
  return {
    version: 1,
    title: value.title,
    ...(description !== undefined && { description }),
    blocks: blocks as RichOutputInputBlock[],
  };
}

type ResolvedChartSnapshot = {
  block_index: number;
  labels: string[];
  series: RichOutputChartSeries[];
};

type RichOutputSnapshot = {
  version: 1;
  resolved_charts: ResolvedChartSnapshot[];
};

function parseResolvedChart(value: unknown): ResolvedChartSnapshot | null {
  if (!isRecord(value) || !hasOnlyKeys(value, ["block_index", "labels", "series"])) return null;
  if (!Number.isInteger(value.block_index) || (value.block_index as number) < 0) return null;
  if ((value.block_index as number) > 3) return null;
  if (!Array.isArray(value.labels) || value.labels.length < 1 || value.labels.length > 100) {
    return null;
  }
  const labels = value.labels;
  if (!labels.every((label) => isBoundedString(label, 1, 120))) return null;
  if (!Array.isArray(value.series) || value.series.length < 1 || value.series.length > 4) {
    return null;
  }
  const series = value.series.map((item) => parseChartSeries(item, labels.length));
  if (series.some((item) => item === null)) return null;
  return {
    block_index: value.block_index as number,
    labels,
    series: series as RichOutputChartSeries[],
  };
}

function parseRichOutputSnapshot(value: unknown): RichOutputSnapshot | null {
  const bytes = serializedByteLength(value);
  if (bytes === null || bytes > MAX_PAYLOAD_BYTES || !isRecord(value)) return null;
  if (!hasOnlyKeys(value, ["version", "resolved_charts"]) || value.version !== 1) return null;
  if (!Array.isArray(value.resolved_charts)) return null;
  if (value.resolved_charts.length < 1 || value.resolved_charts.length > 4) return null;
  const charts = value.resolved_charts.map(parseResolvedChart);
  if (charts.some((chart) => chart === null)) return null;
  return { version: 1, resolved_charts: charts as ResolvedChartSnapshot[] };
}

function expectedCSVSeriesLabels(block: RichOutputCSVChartBlock): string[] {
  return block.csv.series.map((series) => series.label ?? series.column);
}

function resolveCSVChart(
  block: RichOutputCSVChartBlock,
  snapshot: ResolvedChartSnapshot,
): RichOutputChartBlock | null {
  const expectedLabels = expectedCSVSeriesLabels(block);
  if (snapshot.series.length !== expectedLabels.length) return null;
  if (!snapshot.series.every((series, index) => series.label === expectedLabels[index]))
    return null;
  return {
    type: "chart",
    chart_type: block.chart_type,
    title: block.title,
    summary: block.summary,
    labels: snapshot.labels,
    series: snapshot.series,
  };
}

function resolveCSVCharts(input: RichOutputInput, result: unknown): RichOutput | null {
  const csvIndexes = new Set<number>();
  input.blocks.forEach((block, index) => {
    if (block.type === "chart" && "csv" in block) csvIndexes.add(index);
  });
  if (csvIndexes.size === 0) {
    return parseRichOutputSnapshot(result) ? null : (input as RichOutput);
  }
  const snapshot = parseRichOutputSnapshot(result);
  if (!snapshot || snapshot.resolved_charts.length !== csvIndexes.size) return null;
  const byIndex = new Map<number, ResolvedChartSnapshot>();
  for (const chart of snapshot.resolved_charts) {
    if (!csvIndexes.has(chart.block_index) || byIndex.has(chart.block_index)) return null;
    byIndex.set(chart.block_index, chart);
  }
  const blocks: RichOutputBlock[] = [];
  for (let index = 0; index < input.blocks.length; index += 1) {
    const block = input.blocks[index];
    if (block.type !== "chart" || !("csv" in block)) {
      blocks.push(block as RichOutputBlock);
      continue;
    }
    const chartSnapshot = byIndex.get(index);
    if (!chartSnapshot) return null;
    const resolved = resolveCSVChart(block, chartSnapshot);
    if (!resolved) return null;
    blocks.push(resolved);
  }
  return { ...input, blocks };
}

export function parseRichOutput(value: unknown, result?: unknown): RichOutput | null {
  const input = parseRichOutputInput(value);
  return input ? resolveCSVCharts(input, result) : null;
}
