import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MarkdownComment } from "./markdown-comment";
import { ImplicitTaskLinksContext } from "./task-link-context";
vi.mock("@/components/shared/markdown-components", () => ({
  remarkPlugins: [],
  markdownComponents: {},
}));

describe("orchestrator issue links", () => {
  it("does not turn external issue keys into Office links and preserves explicit Jira URLs", () => {
    render(
      <ImplicitTaskLinksContext.Provider value={false}>
        <MarkdownComment content="DEMO-101 and [DEMO-102](https://issues.example.invalid/browse/DEMO-102)" />
      </ImplicitTaskLinksContext.Provider>,
    );
    expect(screen.queryByRole("link", { name: "DEMO-101" })).toBeNull();
    expect(screen.getByRole("link", { name: "DEMO-102" }).getAttribute("href")).toBe(
      "https://issues.example.invalid/browse/DEMO-102",
    );
  });
  it("preserves legacy Office identifier linking outside orchestration", () => {
    render(<MarkdownComment content="KAN-42" />);
    expect(screen.getByRole("link", { name: "KAN-42" }).getAttribute("href")).toBe(
      "/office/tasks/KAN-42",
    );
  });
});
