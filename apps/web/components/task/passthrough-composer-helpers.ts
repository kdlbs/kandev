import type { SlashCommand } from "@/hooks/use-inline-slash";

export type PassthroughSuggestionKind = "command" | "file";

export type PassthroughCommand = Pick<SlashCommand, "id" | "label" | "description"> & {
  agentCommandName: string;
};

export type PassthroughSuggestionState = {
  kind: PassthroughSuggestionKind;
  triggerStart: number;
  query: string;
} | null;

function isTriggerBoundary(text: string, pos: number): boolean {
  if (pos === 0) return true;
  return /\s/.test(text[pos - 1] ?? "");
}

export function detectPassthroughSuggestion(
  text: string,
  cursorPos: number,
): PassthroughSuggestionState {
  const beforeCursor = text.slice(0, cursorPos);
  const lastAt = beforeCursor.lastIndexOf("@");
  if (lastAt >= 0 && isTriggerBoundary(text, lastAt)) {
    const query = beforeCursor.slice(lastAt + 1);
    if (!/\s/.test(query)) return { kind: "file", triggerStart: lastAt, query };
  }

  const lastSlash = beforeCursor.lastIndexOf("/");
  if (lastSlash >= 0 && isTriggerBoundary(text, lastSlash)) {
    const query = beforeCursor.slice(lastSlash + 1);
    if (!/\s/.test(query) && /^[\w-]*$/.test(query)) {
      return { kind: "command", triggerStart: lastSlash, query };
    }
  }
  return null;
}

export function buildPassthroughCommands(
  agentCommands: { name: string; description?: string }[] | undefined,
): PassthroughCommand[] {
  return (agentCommands ?? [])
    .filter((cmd) => !(cmd.description ?? "").includes("(bundled)"))
    .map((cmd) => ({
      id: `agent-${cmd.name}`,
      label: `/${cmd.name}`,
      description: cmd.description || `Run /${cmd.name} command`,
      agentCommandName: cmd.name,
    }));
}

export function filterPassthroughCommands(
  commands: PassthroughCommand[],
  query: string,
): PassthroughCommand[] {
  if (!query) return commands;
  const lower = query.toLowerCase();
  return commands.filter((cmd) => {
    const name = cmd.agentCommandName.toLowerCase();
    return name.startsWith(lower) || cmd.label.toLowerCase().includes(lower);
  });
}

export function replacePassthroughRange(
  value: string,
  start: number,
  end: number,
  insertion: string,
): { value: string; caret: number } {
  const suffix = value.slice(end);
  const normalizedSuffix = /\s$/.test(insertion) && /^\s/.test(suffix) ? suffix.slice(1) : suffix;
  const next = value.slice(0, start) + insertion + normalizedSuffix;
  return { value: next, caret: start + insertion.length };
}

export function fileReferenceToken(path: string): string {
  return `@${path} `;
}
