export type TaskMentionReference = { title: string; taskId: string };

const MAX_SOURCE_BYTES = 262_144;
const MAX_REFERENCES = 200;
const MAX_TITLE_LENGTH = 512;
const MAX_ID_LENGTH = 256;
const RECORD_PREFIX = / \(id: [a-zA-Z0-9_-]{1,256}, workflow_id: [^,]+, step: /;

function parseReference(line: string): TaskMentionReference | null {
  const match = /^- (.+) \(id: ([^,]+), workflow_id: [^,]+, step: .+\)$/.exec(line);
  if (!match) return null;
  const [, title, taskId] = match;
  if (
    title.length > MAX_TITLE_LENGTH ||
    RECORD_PREFIX.test(title) ||
    /[\p{Cc}<>]/u.test(title) ||
    taskId.length > MAX_ID_LENGTH ||
    !/^[a-zA-Z0-9_-]+$/.test(taskId)
  )
    return null;
  return { title, taskId };
}

export function taskMentionReferences(source: string): TaskMentionReference[] {
  if (
    source.length > MAX_SOURCE_BYTES ||
    new TextEncoder().encode(source).length > MAX_SOURCE_BYTES
  ) {
    return [];
  }
  const references = new Map<string, TaskMentionReference>();
  const ambiguous = new Set<string>();
  let recordCount = 0;
  for (const block of source.matchAll(/<kandev-system>([\s\S]*?)<\/kandev-system>/g)) {
    const content = block[1].trim();
    // i18n-exempt: existing agent-facing reference context marker, never displayed.
    if (!content.startsWith("REFERENCED TASKS:") || content.includes("<kandev-system>")) continue;
    for (const line of content.split(/\r?\n/).slice(1)) {
      if (++recordCount > MAX_REFERENCES) return [];
      const reference = parseReference(line);
      if (!reference || ambiguous.has(reference.title)) continue;
      const previous = references.get(reference.title);
      if (previous && previous.taskId !== reference.taskId) {
        references.delete(reference.title);
        ambiguous.add(reference.title);
      } else {
        references.set(reference.title, reference);
      }
    }
  }
  return [...references.values()];
}
