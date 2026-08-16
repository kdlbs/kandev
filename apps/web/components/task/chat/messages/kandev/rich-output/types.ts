export type RichOutputFileBlock = {
  type: "file";
  path: string;
  repo?: string;
  title?: string;
  caption?: string;
  mime_type?: string;
};

export type RichOutputChartSeries = {
  label: string;
  values: Array<number | null>;
};

export type RichOutputChartBlock = {
  type: "chart";
  chart_type: "line" | "bar";
  title: string;
  summary: string;
  labels: string[];
  series: RichOutputChartSeries[];
};

export type RichOutputCSVSeries = {
  column: string;
  label?: string;
};

export type RichOutputCSVSource = {
  path: string;
  repo?: string;
  x_column: string;
  series: RichOutputCSVSeries[];
};

export type RichOutputCSVChartBlock = {
  type: "chart";
  chart_type: "line" | "bar";
  title: string;
  summary: string;
  csv: RichOutputCSVSource;
};

export type RichOutputMetric = {
  label: string;
  value: string;
  detail?: string;
};

export type RichOutputMetricsBlock = {
  type: "metrics";
  items: RichOutputMetric[];
};

export type RichOutputBlock = RichOutputFileBlock | RichOutputChartBlock | RichOutputMetricsBlock;

export type RichOutputInputBlock = RichOutputBlock | RichOutputCSVChartBlock;

export type RichOutputInput = {
  version: 1;
  title: string;
  description?: string;
  blocks: RichOutputInputBlock[];
};

export type RichOutput = {
  version: 1;
  title: string;
  description?: string;
  blocks: RichOutputBlock[];
};
