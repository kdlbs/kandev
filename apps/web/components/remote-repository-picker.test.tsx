import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RemoteRepositoryPickerPanel,
  type RemoteRepositoryPickerPanelProps,
} from "./remote-repository-picker";
import {
  githubSite,
  makeAccessible,
  renderInProvider,
} from "./task-create-dialog-remote-repo-chip-test-support";

const REPO_INPUT = "remote-repo-input";

afterEach(cleanup);

const repos = [
  githubSite({ owner: "kdlbs", name: "kandev", full_name: "kdlbs/kandev" }),
  githubSite({ owner: "acme", name: "api", full_name: "acme/api", private: true }),
];

function renderPanel(overrides: Partial<RemoteRepositoryPickerPanelProps> = {}) {
  const props: RemoteRepositoryPickerPanelProps = {
    accessible: makeAccessible({ repos }),
    onPick: vi.fn(),
    isRepoSelected: () => false,
    isTextSubmittable: () => false,
    onSubmitText: () => ({ status: "ignored" }),
    placeholder: "Search",
    ...overrides,
  };
  renderInProvider(<RemoteRepositoryPickerPanel {...props} />);
  return props;
}

function optionNames() {
  return screen.getAllByTestId("remote-repo-option").map((option) => option.textContent);
}

describe("RemoteRepositoryPickerPanel", () => {
  it("lists accessible repositories without any input", () => {
    renderPanel();
    expect(optionNames()).toEqual([
      expect.stringContaining("kdlbs/kandev"),
      expect.stringContaining("acme/api"),
    ]);
  });

  it("forwards typed text to the repository search", () => {
    const search = vi.fn();
    renderPanel({ accessible: makeAccessible({ repos, search }) });
    fireEvent.change(screen.getByTestId(REPO_INPUT), { target: { value: "kandev" } });
    expect(search).toHaveBeenLastCalledWith("kandev");
  });

  it("reloads the list from the refresh control", () => {
    const refresh = vi.fn();
    renderPanel({ accessible: makeAccessible({ repos, refresh }) });
    fireEvent.click(screen.getByTestId("remote-repo-refresh"));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("disables the refresh control while the list loads", () => {
    renderPanel({ accessible: makeAccessible({ repos, refresh: vi.fn(), loading: true }) });
    expect(screen.getByTestId<HTMLButtonElement>("remote-repo-refresh").disabled).toBe(true);
  });

  it("omits the refresh control when the list cannot refresh", () => {
    renderPanel();
    expect(screen.queryByTestId("remote-repo-refresh")).toBeNull();
  });

  it("hides repositories the caller does not allow", () => {
    renderPanel({ isRepoAllowed: (repo) => repo.owner === "kdlbs" });
    expect(optionNames()).toEqual([expect.stringContaining("kdlbs/kandev")]);
  });

  it("marks selected repositories and reports a pick", () => {
    const props = renderPanel({ isRepoSelected: (repo) => repo.owner === "kdlbs" });
    expect(screen.getAllByTestId("already-added-repository-marker")).toHaveLength(1);
    fireEvent.click(screen.getAllByTestId("remote-repo-option")[1]!);
    expect(props.onPick).toHaveBeenCalledWith(expect.objectContaining({ fullName: "acme/api" }));
  });

  it("submits typed text on Enter and shows a rejection message", () => {
    const onSubmitText = vi.fn(() => ({ status: "rejected" as const, message: "Not allowed" }));
    renderPanel({ onSubmitText, isTextSubmittable: () => true, stagedHint: <span>hint</span> });
    const input = screen.getByTestId(REPO_INPUT);
    fireEvent.change(input, { target: { value: "other/repo" } });
    expect(screen.getByText("hint")).toBeTruthy();
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onSubmitText).toHaveBeenCalledWith("other/repo");
    expect(screen.getByRole("alert").textContent).toBe("Not allowed");
  });

  it("keeps plain Enter inside the picker when the typed text is not submittable", () => {
    renderPanel();
    const input = screen.getByTestId(REPO_INPUT);
    fireEvent.change(input, { target: { value: "kandev" } });
    expect(fireEvent.keyDown(input, { key: "Enter" })).toBe(false);
  });

  it("moves focus between the search input and options with the arrow keys", () => {
    renderPanel();
    const input = screen.getByTestId(REPO_INPUT);
    const options = screen.getAllByTestId("remote-repo-option");
    input.focus();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(document.activeElement).toBe(options[0]);
    fireEvent.keyDown(options[0]!, { key: "ArrowDown" });
    expect(document.activeElement).toBe(options[1]);
    fireEvent.keyDown(options[1]!, { key: "ArrowUp" });
    fireEvent.keyDown(options[0]!, { key: "ArrowUp" });
    expect(document.activeElement).toBe(input);
  });

  it("shows a rejection while the repository list is unavailable", () => {
    renderPanel({
      accessible: makeAccessible({ unavailable: true }),
      onSubmitText: () => ({ status: "rejected", message: "Not allowed here" }),
    });
    const input = screen.getByTestId(REPO_INPUT);
    fireEvent.change(input, { target: { value: "other/tool" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByRole("alert").textContent).toBe("Not allowed here");
  });

  it("can hide submit errors while the repository list is unavailable", () => {
    renderPanel({
      accessible: makeAccessible({ unavailable: true }),
      onSubmitText: () => ({ status: "rejected", message: "Not allowed here" }),
      hideSubmitErrorWhenUnavailable: true,
    });
    const input = screen.getByTestId(REPO_INPUT);
    fireEvent.change(input, { target: { value: "other/tool" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("announces loading through a translated status region", () => {
    renderPanel({ accessible: makeAccessible({ repos: [], loading: true }) });
    expect(screen.getByRole("status").textContent).toMatch(/loading repositories/i);
    expect(screen.queryByLabelText("Loading")).toBeNull();
  });
});
