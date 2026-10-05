import type { PropsWithChildren } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { Repository, Workspace } from "@/lib/types/http";
import type { HydrationState } from "@/lib/state/store";
import { useProjectFormData } from "./agent-project-form-fields";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: vi.fn(),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));
vi.mock("@/app/actions/workspaces", () => ({
  registerRemoteRepositorySelectionAction: vi.fn(),
}));

const WORKSPACE_ID = "workspace-without-repositories" as Workspace["id"];
const FIXTURE_TIMESTAMP = "2026-10-05T00:00:00Z";

const workspace: Workspace = {
  id: WORKSPACE_ID,
  name: "Fixture workspace",
  owner_id: "owner",
  acp_idle_suspension_enabled: false,
  acp_idle_timeout_minutes: 0,
  created_at: FIXTURE_TIMESTAMP,
  updated_at: FIXTURE_TIMESTAMP,
};

const remoteRepository: Repository = {
  id: "repository-1" as Repository["id"],
  workspace_id: WORKSPACE_ID,
  name: "owner/repository",
  source_type: "git",
  local_path: "/tmp/repository",
  provider: "github",
  provider_repo_id: "42",
  provider_owner: "owner",
  provider_name: "repository",
  remote_url: "https://github.com/owner/repository.git",
  default_branch: "main",
  worktree_branch_prefix: "task/",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: FIXTURE_TIMESTAMP,
  updated_at: FIXTURE_TIMESTAMP,
};

function wrapper(initialState?: HydrationState) {
  return function StateWrapper({ children }: PropsWithChildren) {
    return <StateProvider initialState={initialState}>{children}</StateProvider>;
  };
}

function renderClosedProjectFormData(workspaceId: string | null) {
  return renderHook(
    () => ({
      formData: useProjectFormData(false, workspaceId),
      store: useAppStoreApi(),
    }),
    {
      wrapper: wrapper(
        workspaceId ? { workspaces: { items: [workspace], activeId: workspaceId } } : undefined,
      ),
    },
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("mounts a closed form with an active workspace before its repository collection exists", () => {
  const { result } = renderClosedProjectFormData(WORKSPACE_ID);

  expect(result.current.formData.repositories).toEqual([]);
  expect(result.current.formData.repositoriesLoaded).toBe(false);
});

it("mounts a closed form when no workspace is selected", () => {
  const { result } = renderClosedProjectFormData(null);

  expect(result.current.formData.repositories).toEqual([]);
  expect(result.current.formData.repositoriesLoaded).toBe(false);
});

it("observes repositories added to the store after a closed form mounts", () => {
  const { result } = renderClosedProjectFormData(WORKSPACE_ID);

  act(() => {
    result.current.store.getState().setRepositories(WORKSPACE_ID, [remoteRepository]);
  });

  expect(result.current.formData.repositories).toEqual([remoteRepository]);
  expect(result.current.formData.repositoriesLoaded).toBe(true);
});
