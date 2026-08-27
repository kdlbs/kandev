/**
 * Browser tab title composition. Mirrors the Go rule in
 * `apps/backend/internal/webapp/shell.go` (`ComposeTitle`): the shell rewrites
 * `<title>` server-side so the prefixed title is correct from first paint, and
 * this covers the `/api/v1/app-state` boot path, which never renders through
 * the shell.
 *
 * "Kandev" is the product name, not translatable copy, so no `t()` is involved
 * — consistent with the static `<title>` in `index.html`.
 */
const PRODUCT_TITLE = "Kandev";

/** Returns `"<prefix> Kandev"`, or the plain product name for a blank prefix. */
export function composeTitle(prefix: string | undefined): string {
  const trimmed = prefix?.trim();
  return trimmed ? `${trimmed} ${PRODUCT_TITLE}` : PRODUCT_TITLE;
}

/**
 * Applies the operator-configured tab title prefix. A blank prefix leaves the
 * document title alone: the served shell already says "Kandev", and
 * overwriting it would clobber a title some other surface has set.
 */
export function applyTitlePrefix(prefix: string | undefined, doc: Document): void {
  if (!prefix?.trim()) return;
  doc.title = composeTitle(prefix);
}
