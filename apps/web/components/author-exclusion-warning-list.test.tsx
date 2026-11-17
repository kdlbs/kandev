import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { AuthorExclusionWarningList } from "./author-exclusion-warning-list";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

const base = { workflow_name: "Office Review", step_name: "Review", role: "reviewer" };

describe("AuthorExclusionWarningList", () => {
  afterEach(cleanup);

  it("renders nothing without warnings", () => {
    const { container } = render(<AuthorExclusionWarningList warnings={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders one localized item per warning code", () => {
    const warnings: AuthorExclusionWarning[] = [
      { ...base, code: "routing_disabled" },
      { ...base, code: "single_provider", provider_id: "claude-acp" },
      { ...base, code: "agent_single_provider", provider_id: "codex-acp", agent_name: "QA" },
      { code: "check_failed" },
    ];
    const label = (id: string) => (id === "codex-acp" ? "Codex (ACP)" : id);
    render(<AuthorExclusionWarningList warnings={warnings} formatProvider={label} />);
    expect(screen.getByText("Reviewer exclusion setup")).toBeTruthy();
    const items = screen.getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toEqual([
      "Office Review / Review (Reviewer): routing is off, so the exclusion is not enforced.",
      "Office Review / Review (Reviewer): only claude-acp is configured, so review runs wait whenever the work ran on claude-acp.",
      'Office Review / Review (Reviewer): agent "QA" can only use Codex (ACP), so its review runs wait whenever the work ran on Codex (ACP).',
      "The reviewer exclusion setup could not be checked.",
    ]);
  });
});
