/** Keep JSON inside its inline script even when data contains an HTML closing tag. */
export function serializeInlineScriptJSON(value: unknown): string {
  return JSON.stringify(value).replace(/</g, "\\u003c");
}
