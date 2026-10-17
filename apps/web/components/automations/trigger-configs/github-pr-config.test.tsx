import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { GitHubPRConfig } from "./github-pr-config";

vi.mock("@/components/github/repo-filter-selector", () => ({
  RepoFilterSelector: () => null,
}));

const checkedAttribute = "aria-checked";

afterEach(cleanup);

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.19
describe("GitHub PR event selection", () => {
  it("emits the cleared selection and can select Opened again", () => {
    const onUpdate = vi.fn();
    const config = { events: ["opened"], repos: [{ owner: "acme", name: "widget" }] };
    const view = (current: Record<string, unknown>) => (
      <TooltipProvider>
        <GitHubPRConfig config={current} onUpdate={onUpdate} />
      </TooltipProvider>
    );
    const { rerender } = render(view(config));
    const checkbox = () => screen.getByRole("checkbox", { name: "Opened" });
    expect(checkbox().getAttribute(checkedAttribute)).toBe("true");

    fireEvent.click(checkbox());
    expect(onUpdate).toHaveBeenLastCalledWith({ ...config, events: [] });
    const cleared = onUpdate.mock.lastCall![0];
    rerender(view(cleared));
    expect(checkbox().getAttribute(checkedAttribute)).toBe("false");

    fireEvent.click(checkbox());
    expect(onUpdate).toHaveBeenLastCalledWith(config);
    rerender(view(onUpdate.mock.lastCall![0]));
    expect(checkbox().getAttribute(checkedAttribute)).toBe("true");
  });

  it.each([
    ["missing", {}, "false"],
    ["null", { events: null }, "false"],
    ["empty", { events: [] }, "false"],
    ["unsupported only", { events: ["closed"] }, "false"],
    ["mixed", { events: ["closed", "opened"] }, "true"],
  ])("renders %s saved events", (_name, config, checked) => {
    render(
      <TooltipProvider>
        <GitHubPRConfig config={config} onUpdate={vi.fn()} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("checkbox", { name: "Opened" }).getAttribute(checkedAttribute)).toBe(
      checked,
    );
  });
});
