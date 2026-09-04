import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as htmlPreviewDocument from "@/lib/html-preview/html-preview-document";
import { HtmlPreviewContent } from "./html-preview-content";

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderPreview(onTogglePreview = vi.fn()) {
  return {
    onTogglePreview,
    ...render(
      <TooltipProvider>
        <HtmlPreviewContent
          path="reports/index.html"
          content="<h1>Report</h1><style>h1 { color: red; }</style>"
          showExternalVcsLink={false}
          onTogglePreview={onTogglePreview}
        />
      </TooltipProvider>,
    ),
  };
}

describe("HtmlPreviewContent", () => {
  it("renders the current buffer in an opaque-origin sandbox with a source toggle", () => {
    const { onTogglePreview } = renderPreview();
    const frame = screen.getByTestId("html-preview-frame");

    expect(frame.getAttribute("title")).toBe("HTML preview");
    expect(frame.getAttribute("sandbox")).toBe("");
    expect(frame.hasAttribute("allow-same-origin")).toBe(false);
    expect(frame.getAttribute("referrerpolicy")).toBe("no-referrer");
    expect(frame.getAttribute("srcdoc")).toContain("<h1>Report</h1>");
    expect(frame.getAttribute("srcdoc")).toContain("default-src 'none'");
    expect(frame.getAttribute("srcdoc")).toContain("script-src 'none'");

    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });

  it("keeps the source route available when document construction fails", () => {
    vi.spyOn(htmlPreviewDocument, "buildHtmlPreviewDocument").mockImplementation(() => {
      throw new Error("document construction failed");
    });
    const { onTogglePreview } = renderPreview();

    expect(screen.getByTestId("html-preview-error").textContent).toContain(
      "Unable to render HTML preview.",
    );
    expect(screen.queryByTestId("html-preview-frame")).toBeNull();
    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });
});
