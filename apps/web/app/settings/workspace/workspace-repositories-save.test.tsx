import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { repositoryId, workspaceId, type RepositoryScript, type Workspace } from "@/lib/types/http";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { useWorkspaceRepositoriesPage } from "./workspace-repositories-client";
import { WorkspaceRepositoriesSection } from "./workspace-repositories-section";
import {
  areRepositoryScriptsDirty,
  isRepositoryDirty,
  type RepositoryWithScripts,
} from "./workspace-repositories-dirty";

const workspace: Workspace = {
  id: workspaceId("workspace-save"),
  name: "Save workspace",
  owner_id: "owner",
  created_at: "",
  updated_at: "",
  acp_idle_suspension_enabled: false,
  acp_idle_timeout_minutes: 30,
};
const repository: RepositoryWithScripts = {
  id: repositoryId("repository-save"),
  workspace_id: workspace.id,
  name: "Save repository",
  source_type: "local",
  local_path: "/repos/save",
  provider: "",
  provider_repo_id: "",
  provider_host: "",
  provider_owner: "",
  provider_name: "",
  default_branch: "main",
  worktree_branch_prefix: "feature/",
  worktree_branch_template: "{{prefix}}{{title}}-{{rand:5}}",
  pull_before_worktree: true,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: "",
  updated_at: "",
  scripts: [],
};
type Ticket = {
  method: string;
  id: string;
  body: Partial<RepositoryScript>;
  finish: (ok?: boolean) => void;
};
let state: ReturnType<typeof useWorkspaceRepositoriesPage>;
let coordinator: ReturnType<typeof useSettingsSaveCoordinator>;
let tickets: Ticket[];
let stored: Map<string, RepositoryScript>;
let postNames: string[];
let creates: number;
let metadataFails: boolean;
let serial: number;
function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function Harness({ initial }: { initial: RepositoryWithScripts[] }) {
  state = useWorkspaceRepositoriesPage(workspace, initial);
  coordinator = useSettingsSaveCoordinator();
  return <WorkspaceRepositoriesSection workspaceId={workspace.id} readOnly={false} state={state} />;
}
function mount(initial = [repository]) {
  render(
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          <Harness initial={initial} />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
}
function current() {
  return state.repositoryItems[0];
}
function dirty() {
  const repo = current();
  const saved = state.savedRepositoriesById.get(repo.id);
  return isRepositoryDirty(repo, saved) || areRepositoryScriptsDirty(repo, saved);
}
function add(name: string) {
  act(() => state.handleAddRepositoryScript(current().id));
  const script = current().scripts.at(-1)!;
  act(() =>
    state.handleUpdateRepositoryScript(current().id, script.id, { name, command: `echo ${name}` }),
  );
  return script.id;
}
async function stageNew() {
  mount([]);
  act(() => state.openDialog());
  act(() => state.handleManualRepoPathChange("/repos/new"));
  await act(async () => {
    await state.handleValidateManualPath();
  });
  act(() => state.handleConfirmLocalRepository());
}
async function save(expected: number) {
  fireEvent.click(screen.getByRole("button", { name: /^(Save changes|Retry save)$/ }));
  await waitFor(() => expect(tickets).toHaveLength(expected));
}
async function settle(index: number, ok = true) {
  await act(async () => {
    tickets[index].finish(ok);
  });
}
async function failed() {
  await waitFor(() => expect(coordinator.status).toBe("error"));
  expect(screen.getByTestId("settings-floating-save").textContent).toContain("Couldn't save");
  expect(
    (screen.getByRole("button", { name: /^(Save changes|Retry save)$/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
  expect(dirty()).toBe(true);
}
async function clean() {
  await waitFor(() => expect(dirty()).toBe(false));
  expect(coordinator.hasDirty).toBe(false);
}
function repositoryResponse(path: string, method: string, body: Record<string, unknown>) {
  if (method === "PATCH" && path.startsWith("/api/v1/repositories/"))
    return metadataFails
      ? json({ error: "Metadata rejected" }, 500)
      : json({ ...repository, ...body, id: path.split("/").at(-1) });
  if (method === "POST" && path === `/api/v1/workspaces/${workspace.id}/repositories`) {
    creates++;
    return metadataFails
      ? json({ error: "Creation rejected" }, 500)
      : json({ ...repository, ...body, id: `created-repository-${creates}` });
  }
  return null;
}
beforeEach(() => {
  tickets = [];
  stored = new Map();
  postNames = [];
  creates = 0;
  metadataFails = false;
  serial = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(String(init.body)) : {};
      if (path.endsWith("/repositories/validate"))
        return json({ path: "/repos/new", exists: true, is_git: true });
      const metadata = repositoryResponse(path, method, body);
      if (metadata) return metadata;
      if ((method === "POST" && path.endsWith("/scripts")) || path.startsWith("/api/v1/scripts/")) {
        const id = method === "POST" ? `canonical-script-${++serial}` : path.split("/").at(-1)!;
        if (method === "POST") postNames.push(body.name);
        return new Promise<Response>((resolve) =>
          tickets.push({
            method,
            id,
            body,
            finish(ok = true) {
              if (!ok) {
                resolve(json({ error: "Script rejected before persistence" }, 500));
                return;
              }
              if (method === "DELETE") {
                if (!stored.delete(id)) {
                  resolve(json({ error: "Script missing" }, 404));
                  return;
                }
                resolve(json({}));
                return;
              }
              const script: RepositoryScript = {
                created_at: "",
                updated_at: "",
                ...body,
                id,
                repository_id:
                  method === "POST" ? path.split("/").at(-2) : stored.get(id)?.repository_id,
              };
              stored.set(id, script);
              resolve(json(script));
            },
          }),
        );
      }
      throw new Error(`Unexpected ${method} ${path}`);
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  clearNavigationBlockerForTests();
});
// @covers AC-WORKSPACES-REPOSITORY-SAVE-001.1, AC-WORKSPACES-REPOSITORY-SAVE-001.4
it.each(["established", "new"])(
  "retains acknowledged scripts after %s repository Save fails and retries without duplicate identities",
  async (kind) => {
    if (kind === "new") await stageNew();
    else mount();
    add("Build");
    add("Lint");
    await save(2);
    await settle(0);
    await settle(1, false);
    await failed();
    const accepted = [...stored.values()][0];
    expect(current().scripts[0].id).toBe(accepted.id);
    expect(state.savedRepositoriesById.get(current().id)?.scripts).toEqual([accepted]);
    await save(3);
    await settle(2);
    await clean();
    expect(postNames).toEqual(["Build", "Lint", "Lint"]);
    expect([...stored.values()].map((s) => s.name).sort()).toEqual(["Build", "Lint"]);
    expect(current().scripts.map((s) => s.id)).toEqual([...stored.keys()]);
    expect(creates).toBe(kind === "new" ? 1 : 0);
  },
);

// @covers AC-WORKSPACES-REPOSITORY-SAVE-001.2
it("keeps the shared Save pending until a late successful sibling settles", async () => {
  mount();
  add("Build");
  add("Lint");
  await save(2);
  await settle(1, false);
  expect(coordinator.status).toBe("saving");
  fireEvent.click(screen.getByRole("button", { name: "Saving changes" }));
  expect(tickets).toHaveLength(2);
  await settle(0);
  await failed();
  expect(current().scripts[0].id).toBe(tickets[0].id);
});

// @covers AC-WORKSPACES-REPOSITORY-SAVE-001.3
it.each(["established", "new"])(
  "preserves newer %s repository and script edits across partial acknowledgement",
  async (kind) => {
    if (kind === "new") await stageNew();
    else mount();
    const build = add("Build");
    add("Lint");
    await save(2);
    act(() => {
      state.handleUpdateRepository(current().id, { name: "Newer repository" });
      state.handleUpdateRepositoryScript(current().id, build, { command: "echo newer" });
    });
    await settle(0);
    await settle(1, false);
    await failed();
    expect(current().name).toBe("Newer repository");
    expect(current().scripts[0]).toMatchObject({
      id: tickets[0].id,
      command: "echo newer",
      repository_id: current().id,
    });
    expect(state.savedRepositoriesById.get(current().id)?.scripts[0].command).toBe("echo Build");
    await save(4);
    expect(tickets[2].method).toBe("PUT");
    await settle(2);
    await settle(3);
    await clean();
    expect(stored.get(tickets[0].id)?.command).toBe("echo newer");
  },
);

it("retains an acknowledged create removed during Save as a pending deletion", async () => {
  mount();
  const build = add("Build");
  add("Lint");
  await save(2);
  act(() => state.handleDeleteRepositoryScript(current().id, build));
  await settle(0);
  await settle(1, false);
  await failed();
  expect(current().scripts.map((s) => s.name)).toEqual(["Lint"]);
  expect(state.savedRepositoriesById.get(current().id)?.scripts[0].id).toBe(tickets[0].id);
  await save(4);
  expect(tickets[2].method).toBe("DELETE");
  await settle(2);
  await settle(3);
  await clean();
  expect([...stored.values()].map((s) => s.name)).toEqual(["Lint"]);
});

function existingScripts() {
  const scripts = ["Build", "Lint"].map(
    (name, index): RepositoryScript => ({
      id: `saved-${name}`,
      repository_id: repository.id,
      name,
      command: `echo ${name}`,
      position: index,
      created_at: "",
      updated_at: "",
    }),
  );
  scripts.forEach((s) => stored.set(s.id, s));
  return scripts;
}
it("retains partial deletions and retries only the failed deletion", async () => {
  const scripts = existingScripts();
  mount([{ ...repository, scripts }]);
  act(() => scripts.forEach((s) => state.handleDeleteRepositoryScript(repository.id, s.id)));
  await save(2);
  await settle(0);
  await settle(1, false);
  await failed();
  expect(state.savedRepositoriesById.get(repository.id)?.scripts.map((s) => s.id)).toEqual([
    "saved-Lint",
  ]);
  await save(3);
  expect(tickets[2].id).toBe("saved-Lint");
  await settle(2);
  await clean();
  expect(stored.size).toBe(0);
});
it("retains successful updates while a sibling update fails", async () => {
  const scripts = existingScripts();
  mount([{ ...repository, scripts }]);
  act(() =>
    scripts.forEach((s) =>
      state.handleUpdateRepositoryScript(repository.id, s.id, { command: "echo changed" }),
    ),
  );
  await save(2);
  await settle(0);
  await settle(1, false);
  await failed();
  expect(state.savedRepositoriesById.get(repository.id)?.scripts.map((s) => s.command)).toEqual([
    "echo changed",
    "echo Lint",
  ]);
  await save(3);
  expect(tickets[2].id).toBe("saved-Lint");
  await settle(2);
  await clean();
});
it("Reset restores the acknowledged baseline after partial failure without persistence", async () => {
  mount();
  add("Build");
  add("Lint");
  await save(2);
  await settle(0);
  await settle(1, false);
  await failed();
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await clean();
  expect(current().scripts.map((s) => s.id)).toEqual([...stored.keys()]);
  expect(tickets).toHaveLength(2);
});
it.each(["established", "new"])("completes an ordinary %s repository Save", async (kind) => {
  if (kind === "new") await stageNew();
  else mount();
  add("Build");
  add("Lint");
  await save(2);
  await settle(1);
  await settle(0);
  await clean();
  expect(current().scripts.map((s) => s.id)).toEqual([tickets[0].id, tickets[1].id]);
  expect(stored.size).toBe(2);
});
it.each(["established", "new"])("keeps all rejected %s script drafts retryable", async (kind) => {
  if (kind === "new") await stageNew();
  else mount();
  add("Build");
  add("Lint");
  await save(2);
  await settle(0, false);
  await settle(1, false);
  await failed();
  expect(stored.size).toBe(0);
  expect(current().scripts.every((s) => s.id.startsWith("temp-script-"))).toBe(true);
  await save(4);
  await settle(2);
  await settle(3);
  await clean();
  expect(stored.size).toBe(2);
  expect(creates).toBe(kind === "new" ? 1 : 0);
});
it.each(["established", "new"])(
  "does not admit scripts after a rejected %s repository write",
  async (kind) => {
    if (kind === "new") await stageNew();
    else mount();
    add("Build");
    metadataFails = true;
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await failed();
    expect(tickets).toHaveLength(0);
    expect(stored.size).toBe(0);
    metadataFails = false;
    await save(1);
    await settle(0);
    await clean();
  },
);

it("joins overlapping page saves until all operations have settled", async () => {
  mount();
  add("Build");
  add("Lint");
  let first!: Promise<void>;
  let second!: Promise<void>;
  act(() => {
    first = state.handleSaveRepository(current().id);
    second = state.handleSaveRepository(current().id);
  });
  expect(second).toBe(first);
  await waitFor(() => expect(tickets).toHaveLength(2));
  await settle(0);
  await settle(1);
  await act(async () => {
    await Promise.all([first, second]);
  });
  await clean();
  expect(postNames).toEqual(["Build", "Lint"]);
});

it("keeps accepted repository metadata and clean scripts after a sibling failure", async () => {
  const scripts = existingScripts();
  mount([{ ...repository, scripts }]);
  act(() => state.handleUpdateRepository(repository.id, { name: "Accepted metadata" }));
  add("Check");
  await save(1);
  await settle(0, false);
  await failed();
  expect(state.savedRepositoriesById.get(repository.id)?.name).toBe("Accepted metadata");
  expect(state.savedRepositoriesById.get(repository.id)?.scripts).toEqual(scripts);
  expect(tickets.map((ticket) => ticket.method)).toEqual(["POST"]);
  await save(2);
  await settle(1);
  await clean();
});

it("keeps scripts added during new repository Save attached to the canonical repository", async () => {
  await stageNew();
  add("Build");
  add("Lint");
  await save(2);
  add("Format");
  await settle(0);
  await settle(1, false);
  await failed();
  expect(current().scripts.every((script) => script.repository_id === current().id)).toBe(true);
  expect(current().scripts.map((script) => script.name)).toEqual(["Build", "Lint", "Format"]);
  await save(4);
  await settle(2);
  await settle(3);
  await clean();
  expect([...stored.values()].map((script) => script.name).sort()).toEqual([
    "Build",
    "Format",
    "Lint",
  ]);
  expect(creates).toBe(1);
});
