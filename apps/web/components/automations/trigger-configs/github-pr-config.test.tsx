import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { GitHubPRConfig } from "./github-pr-config";

const repoFilterSelector = vi.hoisted(() => vi.fn((_props: { workspaceId?: string }) => null));

vi.mock("@/components/github/repo-filter-selector", () => ({
  RepoFilterSelector: repoFilterSelector,
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
        <GitHubPRConfig config={current} workspaceId="ws-1" onUpdate={onUpdate} />
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
        <GitHubPRConfig config={config} workspaceId="ws-1" onUpdate={vi.fn()} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("checkbox", { name: "Opened" }).getAttribute(checkedAttribute)).toBe(
      checked,
    );
  });
});

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1
describe("GitHub PR repository filter", () => {
  it("passes the workspace to the repository picker", () => {
    render(
      <TooltipProvider>
        <GitHubPRConfig config={{ all_repos: false }} workspaceId="ws-1" onUpdate={vi.fn()} />
      </TooltipProvider>,
    );
    expect(repoFilterSelector.mock.lastCall![0]).toEqual(
      expect.objectContaining({ workspaceId: "ws-1" }),
    );
  });
});

function renderConfig(config: Record<string, unknown>, onUpdate = vi.fn()) {
  const view = (current: Record<string, unknown>) => (
    <TooltipProvider>
      <GitHubPRConfig config={current} workspaceId="ws-1" onUpdate={onUpdate} />
    </TooltipProvider>
  );
  const utils = render(view(config));
  return { onUpdate, rerender: (next: Record<string, unknown>) => utils.rerender(view(next)) };
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.2
describe("GitHub PR follow-up events", () => {
  it.each([
    ["New commits", "new_commits"],
    ["Review requested", "review_requested"],
  ])("toggles %s without touching other events", (label, value) => {
    const config = { events: ["opened", "unknown"], repos: [] };
    const { onUpdate, rerender } = renderConfig(config);
    const checkbox = () => screen.getByRole("checkbox", { name: label });
    expect(checkbox().getAttribute(checkedAttribute)).toBe("false");

    fireEvent.click(checkbox());
    expect(onUpdate).toHaveBeenLastCalledWith({ ...config, events: ["opened", "unknown", value] });
    rerender(onUpdate.mock.lastCall![0]);
    fireEvent.click(checkbox());
    expect(onUpdate).toHaveBeenLastCalledWith({ ...config, events: ["opened", "unknown"] });
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.3
describe("GitHub PR quiet period", () => {
  const quietLabel = /Wait until pushes stop for/;

  it("is shown only when New commits is selected", () => {
    renderConfig({ events: ["opened"] });
    expect(screen.queryByLabelText(quietLabel)).toBeNull();
    cleanup();
    renderConfig({ events: ["new_commits"], quiet_period_minutes: 5 });
    expect((screen.getByLabelText(quietLabel) as HTMLInputElement).value).toBe("5");
  });

  it("saves whole minutes in range and rejects other values", () => {
    const config = { events: ["new_commits"] };
    const { onUpdate } = renderConfig(config);
    const input = screen.getByLabelText(quietLabel) as HTMLInputElement;
    expect(input.value).toBe("0");

    fireEvent.change(input, { target: { value: "15" } });
    fireEvent.blur(input);
    expect(onUpdate).toHaveBeenLastCalledWith({ ...config, quiet_period_minutes: 15 });

    onUpdate.mockClear();
    for (const invalid of ["1441", "-1", "2.5", ""]) {
      fireEvent.change(input, { target: { value: invalid } });
      fireEvent.blur(input);
      expect(input.value).toBe("0");
    }
    expect(onUpdate).not.toHaveBeenCalled();
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.4
describe("GitHub PR reviewer filter", () => {
  it("is shown only when Review requested is selected and parses on blur", () => {
    renderConfig({ events: ["opened"] });
    expect(screen.queryByLabelText(/Reviewers/)).toBeNull();
    cleanup();

    const config = { events: ["review_requested"], reviewers: ["kandev-bot"] };
    const { onUpdate } = renderConfig(config);
    const input = screen.getByLabelText(/Reviewers/) as HTMLInputElement;
    expect(input.value).toBe("kandev-bot");
    fireEvent.change(input, { target: { value: " kandev-bot, acme/reviewers ,," } });
    fireEvent.blur(input);
    expect(onUpdate).toHaveBeenLastCalledWith({
      ...config,
      reviewers: ["kandev-bot", "acme/reviewers"],
    });
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.6
describe("GitHub PR already-open pull requests", () => {
  const includeLabel = /Also run on pull requests already open/;

  it("is shown only when Opened is selected and saves include_existing_prs", () => {
    renderConfig({ events: ["new_commits"] });
    expect(screen.queryByRole("checkbox", { name: includeLabel })).toBeNull();
    cleanup();

    const config = { events: ["opened"] };
    const { onUpdate } = renderConfig(config);
    const checkbox = screen.getByRole("checkbox", { name: includeLabel });
    expect(checkbox.getAttribute(checkedAttribute)).toBe("false");
    fireEvent.click(checkbox);
    expect(onUpdate).toHaveBeenLastCalledWith({ ...config, include_existing_prs: true });
  });
});

describe("GitHub PR event help", () => {
  it("offers each event's help as its own button that leaves the event unchanged", () => {
    const { onUpdate } = renderConfig({ events: ["opened"], repos: [] });
    const help = screen.getByRole("button", { name: "About New commits" });
    expect(help.closest("label")).toBeNull();
    fireEvent.click(help);
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
