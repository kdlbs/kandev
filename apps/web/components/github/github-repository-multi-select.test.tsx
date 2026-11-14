import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";

const mocks = vi.hoisted(() => ({ useRemoteRepositories: vi.fn() }));
vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: mocks.useRemoteRepositories,
}));

import { GitHubRepositoryMultiSelect } from "./github-repository-multi-select";

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("GitHubRepositoryMultiSelect workspace host", () => {
  it.each(["loading", "failed"])(
    "keeps URL entry uncommitted while workspace status is %s",
    (phase) => {
      const accessible: UseRemoteRepositoriesResult = {
        repos: [],
        availableProviders: [],
        loading: phase === "loading",
        error: phase === "failed" ? new Error("Status unavailable") : null,
        unavailable: phase === "failed",
        search: vi.fn(),
      };
      mocks.useRemoteRepositories.mockReturnValue(accessible);
      const onChange = vi.fn();
      const view = () => (
        <GitHubRepositoryMultiSelect workspaceId="ws-1" selected={[]} onChange={onChange} />
      );
      const { rerender } = render(view());
      fireEvent.click(screen.getByTestId("github-repository-add"));
      const input = screen.getByTestId("remote-repo-input");
      fireEvent.change(input, { target: { value: "https://github.com/acme/widget" } });
      fireEvent.keyDown(input, { key: "Enter" });
      expect(onChange).not.toHaveBeenCalled();

      mocks.useRemoteRepositories.mockReturnValue({
        ...accessible,
        availableProviders: ["github"],
        githubHost: "git.company.example",
        loading: false,
        error: null,
        unavailable: false,
      });
      rerender(view());
      fireEvent.keyDown(input, { key: "Enter" });
      expect(onChange).not.toHaveBeenCalled();

      fireEvent.change(input, { target: { value: "https://git.company.example/acme/widget" } });
      fireEvent.keyDown(input, { key: "Enter" });
      expect(onChange).toHaveBeenCalledOnce();
      expect(onChange).toHaveBeenCalledWith([{ owner: "acme", name: "widget" }]);
    },
  );
});
