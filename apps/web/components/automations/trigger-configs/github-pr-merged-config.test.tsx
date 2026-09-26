import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/github/repo-filter-selector", () => ({
  RepoFilterSelector: () => null,
}));

import { GitHubPRMergedConfig } from "./github-pr-merged-config";

function renderConfig(config: Record<string, unknown>) {
  const onUpdate = vi.fn();
  render(<GitHubPRMergedConfig config={config} workspaceId="ws-1" onUpdate={onUpdate} />);
  return onUpdate;
}

afterEach(cleanup);

describe("GitHubPRMergedConfig", () => {
  it("associates the base-branches label with its input", () => {
    // A visually-adjacent Label with no htmlFor/id pairing announces nothing
    // to assistive tech when the input receives focus.
    renderConfig({ all_repos: true });
    const input = screen.getByLabelText("Base branches (comma-separated)");
    expect(input).toBeInstanceOf(HTMLInputElement);
  });

  it("commits every keystroke, not just on blur", () => {
    // Regression guard: a keyboard save shortcut (Cmd+S) fired before the
    // input blurred used to discard whatever was typed.
    const onUpdate = renderConfig({ all_repos: true });
    const input = screen.getByLabelText("Base branches (comma-separated)");

    fireEvent.change(input, { target: { value: "main, release/*" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ base_branches: ["main", "release/*"] }),
    );
    // No blur was fired — proves onChange alone is what committed the value.
  });

  it("warns when no repositories are selected", () => {
    renderConfig({ all_repos: false, repos: [] });
    // getByText throws if the warning is not rendered.
    screen.getByText(
      "No repositories selected; this trigger will not fire. Enable 'All repositories' or add at least one repository.",
    );
  });

  it("does not warn once all_repos is enabled", () => {
    renderConfig({ all_repos: true });
    expect(
      screen.queryByText(
        "No repositories selected; this trigger will not fire. Enable 'All repositories' or add at least one repository.",
      ),
    ).toBeNull();
  });
});
