// i18n-exempt: CSP directives are browser security policy values, not user-facing copy.
export const HTML_PREVIEW_CSP = [
  "default-src 'none'",
  "script-src 'unsafe-inline'",
  "style-src 'unsafe-inline'",
  "img-src data: blob:",
  "media-src data: blob:",
  "font-src data:",
  "connect-src 'none'",
  "frame-src 'none'",
  "child-src 'none'",
  "object-src 'none'",
  "worker-src 'none'",
  "manifest-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "navigate-to 'none'",
].join("; ");

export function buildHtmlPreviewDocument(content: string): string {
  const parsed = new DOMParser().parseFromString(content, "text/html");
  const csp = parsed.createElement("meta");
  csp.httpEquiv = "Content-Security-Policy";
  csp.content = HTML_PREVIEW_CSP;
  parsed.head.prepend(csp);
  return `<!doctype html>${parsed.documentElement.outerHTML}`;
}
