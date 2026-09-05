// i18n-exempt: CSP directives are browser security policy values, not user-facing copy.
export const HTML_PREVIEW_CSP = [
  "default-src 'none'",
  "script-src 'none'",
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
].join("; ");

const XLINK_NAMESPACE = "http://www.w3.org/1999/xlink";

function isSameDocumentFragment(value: string | null): boolean {
  return value?.trimStart().startsWith("#") ?? false;
}

function neutralizeLinkNavigation(element: Element): void {
  const href = element.getAttribute("href");
  if (href !== null && !isSameDocumentFragment(href)) {
    element.removeAttribute("href");
  }

  const namespacedHref = element.getAttributeNS(XLINK_NAMESPACE, "href");
  const legacyNamespacedHref = element.getAttribute("xlink:href");
  const xlinkHref = namespacedHref ?? legacyNamespacedHref;
  if (xlinkHref !== null && !isSameDocumentFragment(xlinkHref)) {
    element.removeAttributeNS(XLINK_NAMESPACE, "href");
    element.removeAttribute("xlink:href");
  }
}

function neutralizeDocumentNavigation(document: Document): void {
  for (const meta of document.querySelectorAll("meta")) {
    if (meta.getAttribute("http-equiv")?.trim().toLowerCase() === "refresh") {
      meta.remove();
    }
  }

  for (const link of document.querySelectorAll("a, area")) {
    neutralizeLinkNavigation(link);
  }
}

export function buildHtmlPreviewDocument(content: string): string {
  const parsed = new DOMParser().parseFromString(content, "text/html");
  neutralizeDocumentNavigation(parsed);
  const csp = parsed.createElement("meta");
  csp.httpEquiv = "Content-Security-Policy";
  csp.content = HTML_PREVIEW_CSP;
  parsed.head.prepend(csp);
  return `<!doctype html>${parsed.documentElement.outerHTML}`;
}
