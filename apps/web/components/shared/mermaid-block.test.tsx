import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MermaidBlock } from "./mermaid-block";

vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
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
