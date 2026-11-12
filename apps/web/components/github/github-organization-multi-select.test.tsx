import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UseGitHubOrgsResult } from "@/hooks/domains/github/use-github-orgs";

const mocks = vi.hoisted(() => ({ useGitHubOrgs: vi.fn(), refresh: vi.fn() }));
vi.mock("@/hooks/domains/github/use-github-orgs", () => ({ useGitHubOrgs: mocks.useGitHubOrgs }));

import {
  GitHubOrganizationMultiSelect,
  type GitHubOrganizationMultiSelectProps,
} from "./github-organization-multi-select";

const ORG_OPTION = "github-organization-option";

const ORG_INPUT = "github-organization-input";

function orgsResult(overrides: Partial<UseGitHubOrgsResult> = {}): UseGitHubOrgsResult {
  return {
    orgs: ["octocat", "kdlbs", "acme"].map((login) => ({ login, avatar_url: "" })),
    loading: false,
    error: null,
    refresh: mocks.refresh,
    ...overrides,
  };
}

beforeEach(() => mocks.useGitHubOrgs.mockReturnValue(orgsResult()));

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderSelect(overrides: Partial<GitHubOrganizationMultiSelectProps> = {}) {
  const onChange = vi.fn();
  render(
    <GitHubOrganizationMultiSelect
      workspaceId="ws-1"
      selected={[]}
      onChange={onChange}
      {...overrides}
    />,
  );
  return onChange;
}

function openPicker() {
  fireEvent.click(screen.getByTestId("github-organization-add"));
}

function optionTexts() {
  return screen.queryAllByTestId(ORG_OPTION).map((o) => o.textContent ?? "");
}

function typeAndEnter(value: string) {
  const input = screen.getByTestId(ORG_INPUT);
  fireEvent.change(input, { target: { value } });
  fireEvent.keyDown(input, { key: "Enter" });
}

describe("GitHubOrganizationMultiSelect", () => {
  it("lists the account and organizations on open and filters by substring", () => {
    renderSelect();
    openPicker();
    expect(mocks.useGitHubOrgs).toHaveBeenCalledWith("ws-1");
    expect(optionTexts()).toEqual(["octocat", "kdlbs", "acme"]);
    fireEvent.change(screen.getByTestId(ORG_INPUT), {
      target: { value: "DL" },
    });
    expect(optionTexts()).toEqual(["kdlbs"]);
  });

  it("adds a picked organization", () => {
    const onChange = renderSelect({ selected: ["acme"] });
    openPicker();
    fireEvent.click(screen.getAllByTestId(ORG_OPTION)[1]!);
    expect(onChange).toHaveBeenCalledWith(["acme", "kdlbs"]);
  });

  it("marks a selected organization and does not add it twice", () => {
    const onChange = renderSelect({ selected: ["KDLBS"] });
    openPicker();
    expect(screen.getAllByTestId("already-added-repository-marker")).toHaveLength(1);
    fireEvent.click(screen.getAllByTestId(ORG_OPTION)[1]!);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("adds a typed login that is not listed and ignores malformed input", () => {
    const onChange = renderSelect();
    openPicker();
    typeAndEnter("kd lbs");
    expect(onChange).not.toHaveBeenCalled();
    typeAndEnter("other-org");
    expect(onChange).toHaveBeenCalledWith(["other-org"]);
  });

  it("hides and rejects organizations the caller does not allow", () => {
    const onChange = renderSelect({ isOrgAllowed: (login) => login === "kdlbs" });
    openPicker();
    expect(optionTexts()).toEqual(["kdlbs"]);
    typeAndEnter("other-org");
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/does not allow this organization/i);
  });

  it("renders selected organizations as removable chips", () => {
    const onChange = renderSelect({ selected: ["kdlbs", "acme"] });
    expect(screen.getAllByTestId("github-organization-chip").map((c) => c.textContent)).toEqual([
      "kdlbs",
      "acme",
    ]);
    fireEvent.click(screen.getByRole("button", { name: /remove kdlbs/i }));
    expect(onChange).toHaveBeenCalledWith(["acme"]);
  });

  it("reloads the organizations from the refresh control", () => {
    renderSelect();
    openPicker();
    fireEvent.click(screen.getByTestId("github-organization-refresh"));
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows loading, error, and empty states", () => {
    mocks.useGitHubOrgs.mockReturnValue(orgsResult({ orgs: [], loading: true }));
    renderSelect();
    openPicker();
    expect(screen.getByText(/loading organizations/i)).toBeTruthy();
    expect(screen.getByTestId<HTMLButtonElement>("github-organization-refresh").disabled).toBe(
      true,
    );
    cleanup();

    mocks.useGitHubOrgs.mockReturnValue(orgsResult({ orgs: [], error: new Error("boom") }));
    renderSelect();
    openPicker();
    expect(screen.getByText(/could not load organizations: boom/i)).toBeTruthy();
    cleanup();

    mocks.useGitHubOrgs.mockReturnValue(orgsResult({ orgs: [] }));
    renderSelect();
    openPicker();
    expect(screen.getByText(/no organizations found/i)).toBeTruthy();
  });

  it("cannot be opened while disabled", () => {
    renderSelect({ disabled: true });
    expect(screen.getByTestId<HTMLButtonElement>("github-organization-add").disabled).toBe(true);
  });

  it("keeps plain Enter inside the picker and supports arrow-key focus", () => {
    renderSelect();
    openPicker();
    const input = screen.getByTestId(ORG_INPUT);
    fireEvent.change(input, { target: { value: "kd lbs" } });
    expect(fireEvent.keyDown(input, { key: "Enter" })).toBe(false);
    fireEvent.change(input, { target: { value: "" } });
    input.focus();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getAllByTestId(ORG_OPTION)[0]);
  });

  it("announces loading through a translated status region", () => {
    mocks.useGitHubOrgs.mockReturnValue(orgsResult({ orgs: [], loading: true }));
    renderSelect();
    openPicker();
    expect(screen.getByRole("status").textContent).toMatch(/loading organizations/i);
    expect(screen.queryByLabelText("Loading")).toBeNull();
  });
});
