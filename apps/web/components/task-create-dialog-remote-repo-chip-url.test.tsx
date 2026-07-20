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
  it("writes a GitHub URL with source=paste on Enter", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://github.com/acme/api" } });
    fireEvent.keyDown(input, { key: "Enter" });
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

  it("commits a pasted GitHub URL immediately", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.paste(input, { clipboardData: { getData: () => URL_FOO_BAR_PR } });
    expect(onURLChange).toHaveBeenCalledWith(URL_FOO_BAR_PR, "paste");
  });

  it("commits a typed GitHub URL on blur", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: "https://github.com/foo/bar/issues/42" } });
    fireEvent.blur(input);
    expect(onURLChange).toHaveBeenCalledWith("https://github.com/foo/bar/issues/42", "paste");
  });

  it("commits a typed GitHub PR URL on Tab", () => {
    const onURLChange = vi.fn();
    renderChip(onURLChange);
    const input = openInput();
    fireEvent.change(input, { target: { value: URL_FOO_BAR_PR } });
    fireEvent.keyDown(input, { key: "Tab" });
    expect(onURLChange).toHaveBeenCalledWith(URL_FOO_BAR_PR, "paste");
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
