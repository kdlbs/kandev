import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ClarificationMarkdown } from "./clarification-markdown";

describe("ClarificationMarkdown", () => {
  it("renders the approved inline marks and prompt lists", () => {
    const { container } = render(
      <ClarificationMarkdown variant="block">
        {"Use `code`, **bold**, *italic*, and ~~removed~~.\n\n1. First\n2. Second"}
      </ClarificationMarkdown>,
    );

    expect(container.querySelector("code")?.textContent).toBe("code");
    expect(container.querySelector("strong")?.textContent).toBe("bold");
    expect(container.querySelector("em")?.textContent).toBe("italic");
    expect(container.querySelector("del")?.textContent).toBe("removed");
    expect(container.querySelector("ol")?.textContent).toContain("Second");
  });

  it("unwraps block syntax in inline mode and makes links passive on request", () => {
    const { container } = render(
      <button type="button">
        <ClarificationMarkdown variant="inline" linkBehavior="passive">
          {"# [Choice](https://example.com)\n\n- nested item"}
        </ClarificationMarkdown>
      </button>,
    );

    expect(container.querySelector("h1, p, ul, ol, li, a")).toBeNull();
    expect(container.textContent).toContain("Choice");
    expect(container.textContent).toContain("nested item");
  });

  it("does not mount unsupported rich content, raw HTML, or unsafe links", () => {
    const { container } = render(
      <ClarificationMarkdown variant="block">
        {
          '# Heading\n\n> Quote\n\n```ts\nconst value = 1\n```\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n![alt](https://example.com/image.png)\n\n<mark data-danger="true">raw html</mark>\n\n[unsafe](javascript:alert(1)) [safe](https://example.com)'
        }
      </ClarificationMarkdown>,
    );

    expect(container.querySelector("h1, blockquote, pre, table, img, input, mark")).toBeNull();
    expect(container.querySelector("code")).toBeNull();
    expect(container.textContent).toContain("const value = 1");
    expect(container.textContent).toContain("raw html");
    expect(container.querySelector("[data-danger]")).toBeNull();

    const links = Array.from(container.querySelectorAll("a"));
    expect(links).toHaveLength(1);
    expect(links[0]?.textContent).toBe("safe");
    expect(links[0]?.getAttribute("href")).toBe("https://example.com");
    expect(links[0]?.getAttribute("target")).toBe("_blank");
    expect(links[0]?.getAttribute("rel")).toBe("noopener noreferrer");
    expect(container.textContent).toContain("unsafe");
  });

  it("keeps repository-relative links passive", () => {
    const { container } = render(
      <ClarificationMarkdown variant="inline">
        {"Read [the guide](README.md)."}
      </ClarificationMarkdown>,
    );

    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("the guide");
  });

  it("renders safe shared context Markdown and keeps tables locally scrollable", () => {
    const { container } = render(
      <ClarificationMarkdown variant="context">
        {
          "A **shared** decision with `code`.\n\n[Context guide](https://example.com/context)\n\n| Option | Owner | Status |\n| --- | --- | --- |\n| Postgres | Platform | Ready |\n\n[unsafe](javascript:alert(1))\n\n# Unsupported heading\n\n> Unsupported quote\n\n<img src=x onerror=alert(1)>"
        }
      </ClarificationMarkdown>,
    );

    expect(container.querySelector("strong")?.textContent).toBe("shared");
    expect(container.querySelector("code")?.textContent).toBe("code");
    expect(container.querySelector("table th")?.textContent).toBe("Option");
    expect(container.querySelector("table td")?.textContent).toBe("Postgres");
    expect(container.querySelector("[data-clarification-context-table-scroll]")).not.toBeNull();
    expect(
      container.querySelector<HTMLAnchorElement>('a[href="https://example.com/context"]')?.target,
    ).toBe("_blank");
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
    expect(container.querySelector("h1, blockquote, img, pre")).toBeNull();
    expect(container.querySelector("[onerror]")).toBeNull();
    expect(container.textContent).toContain("Unsupported heading");
    expect(container.textContent).toContain("Unsupported quote");
  });
});
