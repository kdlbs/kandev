import type { Spec } from "@json-render/core";
import { resolveComposition } from "./csv-snapshots";
import { decodeLegacyRichOutput } from "./legacy";
import { validateComposition } from "./validate";

export type RichOutputPresentation = { title: string; description?: string; spec: Spec };
function normalizeLegacy(value: unknown, result: unknown): RichOutputPresentation | null {
  const legacy = decodeLegacyRichOutput(value, result);
  if (!legacy) return null;
  const children = legacy.blocks.map((_, i) => `legacy-block-${i}`);
  const elements: Spec["elements"] = {
    report: { type: "Stack", props: { direction: "vertical" }, children },
  };
  legacy.blocks.forEach((block, i) => {
    const { type, ...props } = block;
    const id = children[i];
    if (type === "file") {
      const { mime_type: _unused, ...fileProps } = props as typeof block;
      elements[id] = { type: "FilePreview", props: fileProps, children: [] };
    } else {
      // i18n-exempt: Persisted component identifiers for catalog dispatch, never displayed.
      elements[id] = { type: type === "chart" ? "Chart" : "Metrics", props, children: [] };
    }
  });
  return {
    title: legacy.title,
    description: legacy.description,
    spec: { root: "report", elements },
  };
}

export function normalizeRichOutput(
  value: unknown,
  result?: unknown,
): RichOutputPresentation | null {
  if (typeof value === "object" && value !== null && "version" in value && value.version === 1)
    return normalizeLegacy(value, result);
  const input = validateComposition(value);
  return input ? resolveComposition(input, result) : null;
}
