import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { GitHubCLIForm } from "./github-cli-form";

const mocks = vi.hoisted(() => ({
  fetchAccounts: vi.fn(),
  setConnection: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubCLIAccounts: mocks.fetchAccounts,
  setGitHubWorkspaceConnection: mocks.setConnection,
}));

beforeEach(() => {
  mocks.fetchAccounts.mockReset();
  mocks.setConnection.mockReset();
});

afterEach(() => cleanup());

describe("GitHubCLIForm", () => {
  it("shows account discovery failures instead of an empty-account message", async () => {
    mocks.fetchAccounts.mockRejectedValue(new Error("Request failed (500): github_internal_error"));

    render(
      <ToastProvider>
        <GitHubCLIForm workspaceId="workspace-1" onSaved={vi.fn()} />
      </ToastProvider>,
    );

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Could not load GitHub CLI accounts. Request failed (500): github_internal_error",
    );
    expect(screen.queryByText(/Sign in with/)).toBeNull();
  });

  it("keeps sign-in guidance for a successful empty account list", async () => {
    mocks.fetchAccounts.mockResolvedValue([]);

    render(
      <ToastProvider>
        <GitHubCLIForm workspaceId="workspace-1" onSaved={vi.fn()} />
      </ToastProvider>,
    );

    expect(await screen.findByText(/Sign in with/)).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
