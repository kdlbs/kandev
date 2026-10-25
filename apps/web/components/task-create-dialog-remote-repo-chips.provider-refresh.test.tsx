import { useCallback, useEffect, useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "./state-provider";
import { RemoteRepoChipsRow } from "./task-create-dialog-remote-repo-chips";
import type { DialogFormState, TaskRemoteRepoRow } from "./task-create-dialog-types";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { pluginRegistry } from "@/lib/plugins/registry";

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  fetchGitHubStatus: vi.fn(async () => ({ authenticated: false, token_configured: false })),
}));
vi.mock("@/lib/api/domains/gitlab-api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  fetchGitLabStatus: vi.fn(async () => null),
}));
vi.mock("@/lib/api/domains/azure-devops-api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  getAzureDevOpsConfig: vi.fn(async () => ({ hasSecret: false, lastOk: false })),
}));

const PLUGIN = "native-chips-refresh-test";
const HOST = "https://code.example.test";
const WORKSPACE = "native-chips-workspace";
const ALPHA = `${HOST}/repos/alpha`;
const BETA = `${HOST}/repos/beta`;
const ROW_STATE = "selected-repositories";
const BRANCH_TRIGGER = "remote-branch-chip-trigger";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
});

function NativeRemoteRows({ restored }: { restored: boolean }) {
  const [remoteRepos, setRemoteRepos] = useState<TaskRemoteRepoRow[]>([
    { key: "alpha", url: "", branch: "", source: "paste" },
    { key: "beta", url: "", branch: "", source: "paste" },
  ]);
  const branchesByUrl = useBranchesByURL(WORKSPACE);
  const prInfoByUrl = usePRInfoByURL(WORKSPACE);
  useEffect(() => {
    if (!restored) return;
    setRemoteRepos([
      { key: "alpha", url: ALPHA, branch: "", source: "paste" },
      { key: "beta", url: BETA, branch: "", source: "paste" },
    ]);
  }, [restored]);
  const onUpdateRow = useCallback((key: string, update: Partial<TaskRemoteRepoRow>) => {
    setRemoteRepos((rows) => rows.map((row) => (row.key === key ? { ...row, ...update } : row)));
  }, []);
  const fs = { remoteRepos, branchesByUrl, prInfoByUrl } as DialogFormState;
  return (
    <>
      <RemoteRepoChipsRow
        workspaceId={WORKSPACE}
        fs={fs}
        onUpdateRow={onUpdateRow}
        onAddRow={() => undefined}
        onRemoveRow={() => undefined}
      />
      <output data-testid={ROW_STATE}>{JSON.stringify(remoteRepos)}</output>
    </>
  );
}

function selectedRows(): TaskRemoteRepoRow[] {
  return JSON.parse(screen.getByTestId(ROW_STATE).textContent ?? "[]");
}

function renderRows(restored: boolean) {
  return (
    <StateProvider>
      <TooltipProvider>
        <NativeRemoteRows restored={restored} />
      </TooltipProvider>
    </StateProvider>
  );
}

function registerProvider() {
  pluginRegistry.forPlugin(PLUGIN).registerRepositoryProvider({
    id: "native-test",
    label: "Native test",
    listRepositories: async () => [],
    matchesURL: (url) => url.startsWith(HOST),
    inspectURL: async ({ url }) => {
      const name = url.split("/").at(-1)!;
      return {
        providerId: "native-test",
        providerHost: HOST,
        ownerOrProject: "acme",
        repositoryId: `${name}-id`,
        repositoryName: name,
        cloneUrl: `${HOST}/acme/${name}.git`,
        defaultBranch: `${name}-default`,
      };
    },
    listBranches: async ({ repository }) => [
      { name: `${repository.repositoryName}-default` },
      { name: `${repository.repositoryName}-feature` },
    ],
  });
}

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.9
// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.10
it("hydrates both pasted repositories and their native branch pickers after late provider boot", async () => {
  const { rerender } = render(renderRows(false));
  rerender(renderRows(true));
  const chips = screen.getAllByTestId("remote-repo-chip");
  expect(chips).toHaveLength(2);
  for (const chip of chips)
    expect(within(chip).getByTestId(BRANCH_TRIGGER).hasAttribute("disabled")).toBe(true);
  act(registerProvider);
  await waitFor(() => {
    expect(selectedRows()).toEqual([
      expect.objectContaining({
        key: "alpha",
        providerRepoId: "alpha-id",
        providerName: "alpha",
        remoteUrl: `${HOST}/acme/alpha.git`,
        branch: "alpha-default",
      }),
      expect.objectContaining({
        key: "beta",
        providerRepoId: "beta-id",
        providerName: "beta",
        remoteUrl: `${HOST}/acme/beta.git`,
        branch: "beta-default",
      }),
    ]);
  });
  const alphaTrigger = within(chips[0]!).getByTestId(BRANCH_TRIGGER);
  const betaTrigger = within(chips[1]!).getByTestId(BRANCH_TRIGGER);
  expect(alphaTrigger.hasAttribute("disabled")).toBe(false);
  expect(betaTrigger.hasAttribute("disabled")).toBe(false);
  fireEvent.click(alphaTrigger);
  fireEvent.click(await screen.findByRole("option", { name: "alpha-feature remote" }));
  await waitFor(() => expect(selectedRows()[0]?.branch).toBe("alpha-feature"));
  fireEvent.click(betaTrigger);
  fireEvent.click(await screen.findByRole("option", { name: "beta-feature remote" }));
  await waitFor(() => expect(selectedRows()[1]?.branch).toBe("beta-feature"));
  expect(selectedRows()[0]?.branch).toBe("alpha-feature");
});
