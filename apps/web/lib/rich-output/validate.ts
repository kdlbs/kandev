import { z } from "zod";
import {
  catalog,
  componentProps,
  type ComponentName,
  type TableProps,
  type ChartProps,
} from "./catalog";

const elementId = z
  .string()
  .regex(/^[A-Za-z][A-Za-z0-9_-]{0,63}$/)
  .refine((id) => !["constructor", "prototype", "__proto__"].includes(id));
const elementSchema = z.strictObject({
  type: z.enum(Object.keys(componentProps) as [ComponentName, ...ComponentName[]]),
  props: z.record(z.string(), z.unknown()),
  children: z.array(elementId).max(64),
});
const inputSchema = z.strictObject({
  version: z.literal(2),
  catalog_version: z.literal(1),
  title: z.string().refine((s) => Array.from(s).length >= 1 && Array.from(s).length <= 120),
  description: z
    .string()
    .refine((s) => Array.from(s).length <= 500)
    .optional(),
  spec: z.strictObject({ root: elementId, elements: z.record(elementId, elementSchema) }),
});
export type CompositionInput = z.infer<typeof inputSchema>;
export type CompositionElement = CompositionInput["spec"]["elements"][string];

export function withinPayloadBound(value: unknown): boolean {
  try {
    const encoded = JSON.stringify(value);
    return encoded !== undefined && new TextEncoder().encode(encoded).length <= 64 * 1024;
  } catch {
    return false;
  }
}

export function isWorkspaceRelativePath(value: string): boolean {
  if (value.trim() === "" || value.includes("\0")) return false;
  const normalized = value.replaceAll("\\", "/");
  if (normalized.startsWith("/") || /:\/\/|^data:|^[a-z]:/i.test(normalized)) return false;
  return (
    !normalized.split("/").includes("..") &&
    normalized.split("/").some((part) => part !== "" && part !== ".")
  );
}

function validContent(element: CompositionElement): boolean {
  const parsed = componentProps[element.type].safeParse(element.props);
  if (!parsed.success) return false;
  if (element.type === "FilePreview") return isWorkspaceRelativePath(element.props.path as string);
  if (element.type === "Table") {
    const table = element.props as TableProps;
    if (new Set(table.columns.map((column) => column.key)).size !== table.columns.length)
      return false;
    if (!("csv" in element.props))
      return table.rows.every((row) => row.length === table.columns.length);
  }
  if (element.type !== "Chart" && element.type !== "Table") return true;
  if ("csv" in element.props) {
    const source = element.props.csv as { path: string };
    return isWorkspaceRelativePath(source.path) && source.path.toLowerCase().endsWith(".csv");
  }
  const chart = element.props as ChartProps;
  return chart.series.every((series) => series.values.length === chart.labels.length);
}

function validTree(input: CompositionInput): boolean {
  const elements = input.spec.elements;
  const entries = Object.entries(elements);
  if (entries.length < 1 || entries.length > 64) return false;
  if (
    entries.filter(([, e]) =>
      ["Chart", "Metrics", "FilePreview", "Table", "StatusList"].includes(e.type),
    ).length > 4
  )
    return false;
  const seen = new Set<string>();
  function visit(id: string, depth: number): boolean {
    const element = elements[id];
    if (!Object.hasOwn(elements, id) || !element || seen.has(id) || depth > 8) return false;
    seen.add(id);
    const container = element.type === "Stack" || element.type === "Group";
    if (container ? element.children.length === 0 : element.children.length !== 0) return false;
    return validContent(element) && element.children.every((child) => visit(child, depth + 1));
  }
  return visit(input.spec.root, 1) && seen.size === entries.length;
}

export function validateComposition(value: unknown): CompositionInput | null {
  if (!withinPayloadBound(value)) return null;
  const parsed = inputSchema.safeParse(value);
  if (!parsed.success || !validTree(parsed.data) || !catalog.validate(parsed.data.spec).success)
    return null;
  return parsed.data;
}
