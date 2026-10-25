import type { PromptMentionSegment } from "@/lib/prompts/prompt-mention-segments";
import type { TaskMentionReference } from "./task-mention-references";
import { unified, type Plugin } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import { remarkMathCompat } from "@/lib/markdown/remark-math-compat";

const parser = unified()
  .use(remarkParse)
  .use(remarkGfm)
  .use(remarkMath)
  .use(remarkMathCompat as Plugin);

type SourceNode = {
  type: string;
  children?: SourceNode[];
  position?: { start: { offset?: number }; end: { offset?: number } };
};

const EMPTY_TOKENS: ReadonlyMap<string, TaskMentionReference> = new Map();
const PROTECTED_TYPES = new Set([
  "code",
  "inlineCode",
  "link",
  "linkReference",
  "image",
  "imageReference",
  "html",
  "math",
  "inlineMath",
]);

function protectedRanges(node: SourceNode, ranges: Array<{ start: number; end: number }>) {
  if (PROTECTED_TYPES.has(node.type)) {
    const start = node.position?.start.offset;
    const end = node.position?.end.offset;
    if (start !== undefined && end !== undefined) ranges.push({ start, end });
    return;
  }
  for (const child of node.children ?? []) protectedRanges(child, ranges);
}

function unusedTokenPrefix(source: string) {
  let salt = 0;
  while (source.includes(`KANDEVTASKREF${salt}X`)) salt++;
  return `KANDEVTASKREF${salt}X`;
}

export function protectTaskMentionMarkdown(
  source: string,
  segments: readonly PromptMentionSegment[],
  references: readonly TaskMentionReference[],
): { content: string; tokens: ReadonlyMap<string, TaskMentionReference> } {
  if (!segments.some((segment) => segment.kind === "prompt")) {
    return { content: source, tokens: EMPTY_TOKENS };
  }
  const ranges: Array<{ start: number; end: number }> = [];
  protectedRanges(parser.runSync(parser.parse(source), source), ranges);
  const byTitle = new Map(references.map((reference) => [reference.title, reference]));
  const tokenByTitle = new Map<string, string>();
  const tokens = new Map<string, TaskMentionReference>();
  const prefix = unusedTokenPrefix(source);
  let offset = 0;
  const content = segments
    .map((segment) => {
      const start = offset;
      offset += segment.value.length;
      if (
        segment.kind !== "prompt" ||
        ranges.some((range) => start >= range.start && start < range.end)
      ) {
        return segment.value;
      }
      const reference = byTitle.get(segment.name);
      if (!reference) return segment.value;
      const token = tokenByTitle.get(segment.name) ?? `${prefix}${tokens.size}Z`;
      tokenByTitle.set(segment.name, token);
      tokens.set(token, reference);
      return `@${token}`;
    })
    .join("");
  return { content, tokens };
}
