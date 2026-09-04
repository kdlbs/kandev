import { describe, expect, it } from "vitest";
import { buildHtmlPreviewDocument, HTML_PREVIEW_CSP } from "./html-preview-document";

describe("buildHtmlPreviewDocument", () => {
  it("injects the restrictive CSP before the source head content", () => {
    const document = buildHtmlPreviewDocument(
      "<!doctype html><html><head><title>Example</title></head><body><h1>Hello</h1></body></html>",
    );

    expect(document).toContain(
      `<meta http-equiv="Content-Security-Policy" content="${HTML_PREVIEW_CSP}">`,
    );
    expect(document.indexOf(HTML_PREVIEW_CSP)).toBeLessThan(document.indexOf("Example"));
  });

  it("preserves markup while denying active capabilities", () => {
    const document = buildHtmlPreviewDocument(
      '<style>h1 { color: red; }</style><script>document.body.dataset.ready = "yes";</script><img src="data:image/png;base64,abc"><img src="https://example.com/image.png">',
    );

    expect(document).toContain("h1 { color: red; }");
    expect(document).toContain('document.body.dataset.ready = "yes";');
    expect(document).toContain("script-src 'none'");
    expect(document).not.toContain("script-src 'unsafe-inline'");
    expect(document).toContain("img-src data: blob:");
    expect(document).toContain("font-src data:");
    expect(document).toContain("default-src 'none'");
    expect(document).toContain("connect-src 'none'");
    expect(document).toContain("frame-src 'none'");
    expect(document).toContain("object-src 'none'");
    expect(document).toContain("worker-src 'none'");
    expect(document).toContain("manifest-src 'none'");
    expect(document).toContain("form-action 'none'");
    expect(document).toContain("base-uri 'none'");
    expect(document).not.toContain("navigate-to");
  });

  it("returns browser-parsed markup for incomplete documents", () => {
    const document = buildHtmlPreviewDocument("<h1>Incomplete");

    expect(document).toContain("<h1>Incomplete</h1>");
    expect(document).toContain("Content-Security-Policy");
  });
});
