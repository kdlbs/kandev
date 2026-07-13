export type PromptMentionSegment =
  | { kind: "text"; value: string }
  | { kind: "prompt"; value: string; name: string };

import { buildPromptMentionNames, matchPromptMention } from "./prompt-mention-parser";

export { buildPromptMentionNames } from "./prompt-mention-parser";

export function splitPromptMentionSegments(
  content: string,
  promptNames: string[],
): PromptMentionSegment[] {
  return splitPreparedPromptMentionSegments(content, buildPromptMentionNames(promptNames));
}

export function splitPreparedPromptMentionSegments(
  content: string,
  promptNames: string[],
): PromptMentionSegment[] {
  if (content.length === 0 || promptNames.length === 0) return [{ kind: "text", value: content }];

  const segments: PromptMentionSegment[] = [];
  let lastIndex = 0;
  for (let index = 0; index < content.length; ) {
    const match = matchPromptMention(content, index, promptNames);
    if (!match) {
      index += 1;
      continue;
    }

    if (match.start > lastIndex) {
      segments.push({ kind: "text", value: content.slice(lastIndex, match.start) });
    }
    segments.push({
      kind: "prompt",
      value: content.slice(match.start, match.end),
      name: match.name,
    });
    index = match.end;
    lastIndex = match.end;
  }

  if (lastIndex < content.length) {
    segments.push({ kind: "text", value: content.slice(lastIndex) });
  }
  return segments;
}
