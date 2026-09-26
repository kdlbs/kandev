import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";
import { pluginRegistry } from "@/lib/plugins/registry";
import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";
import {
  noopBranch,
  noopRemove,
  renderInProvider,
  row,
} from "./task-create-dialog-remote-repo-chip-test-support";

const PLUGIN_ID = "remote-repo-chip-icon-test";

function PluginProviderIcon({ className }: { className?: string }) {
  return <svg className={className} data-testid="plugin-provider-icon" />;
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("RemoteRepoChip registered provider icon", () => {
  it("keeps the registered icon after picker selection", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      icon: PluginProviderIcon,
      listRepositories: async () => [],
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
    });
    const accessibleRepos: UseRemoteRepositoriesResult = {
      repos: [
        {
          provider: "bitbucket",
          id: "{repository-uuid}",
          owner: "acme",
          name: "site",
          fullName: "acme/site",
          url: "https://bitbucket.org/acme/site.git",
          defaultBranch: "main",
          private: true,
        },
      ],
      availableProviders: ["bitbucket"],
      loading: false,
      unavailable: false,
      error: null,
      search: () => undefined,
    };

    renderInProvider(
      <RemoteRepoChip
        row={row({
          url: "https://bitbucket.org/acme/site.git",
          source: "picker",
          provider: "bitbucket",
          fullName: "acme/site",
        })}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );

    expect(
      screen
        .getByTestId("remote-repo-chip-trigger")
        .contains(screen.getByTestId("plugin-provider-icon")),
    ).toBe(true);
  });
});
