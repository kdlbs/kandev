export type CursorPage<T> = {
  items?: T[];
  next_cursor?: string;
};

export async function collectCursorPages<T>(
  loadPage: (cursor?: string) => Promise<CursorPage<T>>,
  maxPages = 100,
): Promise<T[]> {
  const items: T[] = [];
  const seenCursors = new Set<string>();
  let cursor: string | undefined;
  for (let pageIndex = 0; pageIndex < maxPages; pageIndex += 1) {
    const page = await loadPage(cursor);
    items.push(...(page.items ?? []));
    const nextCursor = page.next_cursor;
    if (!nextCursor || seenCursors.has(nextCursor)) return items;
    seenCursors.add(nextCursor);
    cursor = nextCursor;
  }
  return items;
}
