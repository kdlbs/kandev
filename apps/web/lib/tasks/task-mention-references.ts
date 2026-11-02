export type TaskMentionReference = { title: string; taskId: string };

const MAX_SOURCE_BYTES = 262_144;
const MAX_REFERENCES = 200;
const MAX_TITLE_LENGTH = 512;
const MAX_ID_LENGTH = 256;
const MAX_LINE_LENGTH = 4_096;
const OPEN_TAG = "<kandev-system>";
const CLOSE_TAG = "</kandev-system>";
const RECORD_PREFIX = / \(id: [a-zA-Z0-9_-]{1,256}, workflow_id: [^,]+, step: /;

function parseReference(line: string): TaskMentionReference | null {
  if (line.length > MAX_LINE_LENGTH) return null;
  const match = /^- (.+) \(id: ([a-zA-Z0-9_-]{1,256}), workflow_id: [^,]+, step: .+\)$/.exec(line);
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

function* systemBlocks(source: string): Generator<string> {
  let from = 0;
  for (;;) {
    const open = source.indexOf(OPEN_TAG, from);
    if (open === -1) return;
    const bodyStart = open + OPEN_TAG.length;
    const close = source.indexOf(CLOSE_TAG, bodyStart);
    if (close === -1) return;
    yield source.slice(bodyStart, close);
    from = close + CLOSE_TAG.length;
  }
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
  for (const body of systemBlocks(source)) {
    const content = body.trim();
    // i18n-exempt: existing agent-facing reference context marker, never displayed.
    if (!content.startsWith("REFERENCED TASKS:") || content.includes(OPEN_TAG)) continue;
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
