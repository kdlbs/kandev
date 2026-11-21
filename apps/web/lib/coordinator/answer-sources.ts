/**
 * True only for a reference that parses as an absolute URL whose scheme is
 * exactly http or https. Anything else (a file path, a `javascript:` URL, text
 * that does not parse) is shown as text and never becomes a link.
 */
export function isHttpUrl(ref: string): boolean {
  try {
    const { protocol } = new URL(ref);
    return protocol === "http:" || protocol === "https:";
  } catch {
    return false;
  }
}
