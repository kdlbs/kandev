import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";

const TRIGGER_TID = "remote-repo-chip-trigger";
const INPUT_TID = "remote-repo-input";
const URL_FOO_BAR_PR = "https://github.com/foo/bar/pull/42";

afterEach(() => cleanup());

function row(): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste" };
}

function accessibleRepos(): UseRemoteRepositoriesResult {
  return {
    repos: [],
    availableProviders: [],
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
  };
}

function renderChip(onURLChange: ComponentProps<typeof RemoteRepoChip>["onURLChange"]) {
  return render(
    <TooltipProvider>
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos()}
        onURLChange={onURLChange}
        onBranchChange={() => undefined}
        onRemove={() => undefined}
      />
    </TooltipProvider>,
  );
}

function openInput(): HTMLInputElement {
  fireEvent.click(screen.getByTestId(TRIGGER_TID));
  return screen.getByTestId(INPUT_TID) as HTMLInputElement;
}

describe("RemoteRepoChip URL entry", () => {
  it("commits a trimmed GitHub URL exactly once on plain Enter", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: " https://github.com/acme/api " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onURLChange).toHaveBeenCalledTimes(1);
    expect(onURLChange).toHaveBeenCalledWith("https://github.com/acme/api", "paste");
  });

  it("accepts a supported SSH repository URL", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "git@gitlab.com:acme/api.git" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onURLChange).toHaveBeenCalledWith("git@gitlab.com:acme/api.git", "paste");
  });

  it("does not submit URL input during IME composition or modified Enter", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://github.com/acme/api" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });

    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("keeps a pasted GitHub URL editable until Enter", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.paste(input, { clipboardData: { getData: () => URL_FOO_BAR_PR } });
    expect(input.value).toBe(URL_FOO_BAR_PR);
    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("keeps a typed GitHub URL editable on blur", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://github.com/foo/bar/issues/42" } });
    fireEvent.blur(input);
    expect(input.value).toBe("https://github.com/foo/bar/issues/42");
    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("keeps a typed GitHub PR URL editable on Tab", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: URL_FOO_BAR_PR } });
    fireEvent.keyDown(input, { key: "Tab" });
    expect(input.value).toBe(URL_FOO_BAR_PR);
    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("shows the Remote URL Enter hint for URL-shaped input", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://github.com/foo/bar" } });

    expect(screen.getByText("Remote URL")).toBeTruthy();
    expect(screen.getByText(/press Enter to submit it/i)).toBeTruthy();
    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("surfaces an inline error for an unsupported provider URL", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://bitbucket.org/acme/api" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByRole("alert").textContent).toContain("GitHub, GitLab, or Azure DevOps");
    expect(onURLChange).not.toHaveBeenCalled();
  });
});
