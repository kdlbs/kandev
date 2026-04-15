import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MermaidBlock } from "./mermaid-block";
import { cleanupMermaidOrphans } from "./mermaid-utils";

vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn(), updateToast: vi.fn(), dismissToast: vi.fn() }),
}));

describe("MermaidBlock", () => {
  it("renders a constrained outer container with a horizontal scroll viewport", () => {
    const html = renderToStaticMarkup(<MermaidBlock code={"graph LR\nA-->B"} />);

    expect(html).toContain("mermaid-block");
    expect(html).toContain("block w-full max-w-full min-w-0");
    expect(html).toContain("mermaid-scroll-region w-full overflow-x-auto overflow-y-hidden");
    expect(html).not.toContain("inline-block");
  });
});

const RENDER_ID_1 = "mermaid-test-1";
const RENDER_ID_2 = "mermaid-test-2";
const D_RENDER_ID_2 = `d${RENDER_ID_2}`;

describe("cleanupMermaidOrphans", () => {
  it("removes elements matching the render id", () => {
    const el = document.createElement("div");
    el.id = RENDER_ID_1;
    document.body.appendChild(el);

    expect(document.getElementById(RENDER_ID_1)).not.toBeNull();
    cleanupMermaidOrphans(RENDER_ID_1);
    expect(document.getElementById(RENDER_ID_1)).toBeNull();
  });

  it("removes elements matching the d-prefixed id", () => {
    const el = document.createElement("div");
    el.id = D_RENDER_ID_2;
    document.body.appendChild(el);

    expect(document.getElementById(D_RENDER_ID_2)).not.toBeNull();
    cleanupMermaidOrphans(RENDER_ID_2);
    expect(document.getElementById(D_RENDER_ID_2)).toBeNull();
  });

  it("does nothing when no matching elements exist", () => {
    expect(() => cleanupMermaidOrphans("nonexistent-id")).not.toThrow();
  });
});
