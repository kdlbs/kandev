import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { PromptSyncConfig, PromptSyncConflict } from "@/lib/types/prompt-sync";

const mocks = vi.hoisted(() => ({
  get: vi.fn<() => Promise<PromptSyncConfig | null>>(),
  kept: vi.fn<() => Promise<unknown>>(),
  del: vi.fn<(removal?: unknown) => Promise<unknown>>(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));
vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  getPromptSyncConfig: () => mocks.get(),
  getPromptSyncKeptVersions: () => mocks.kept(),
  deletePromptSyncConfig: (removal?: unknown) => mocks.del(removal),
  setPromptSyncConfig: vi.fn(),
  checkPromptSync: vi.fn(),
  runPromptSync: vi.fn(),
}));

import { usePromptSync } from "./use-prompt-sync";

const config = {
  workspace_id: "ws-1",
  provider: "gitlab",
  project_path: "acme/prompts",
  branch: "main",
  path: ".kandev/prompts",
  interval_seconds: 300,
  poll_enabled: true,
  last_ok: true,
} as PromptSyncConfig;

const kept = (name: string, inUse: "local" | "repository" = "repository"): PromptSyncConflict => ({
  name,
  path: `.kandev/prompts/${name}.md`,
  local_content: "Mine",
  repository_content: "Repo",
  in_use: inUse,
});

async function renderConfigured() {
  const hook = renderHook(() => usePromptSync("ws-1"));
  await waitFor(() => expect(hook.result.current.config).not.toBeNull());
  return hook;
}

describe("usePromptSync removal review", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockResolvedValue(config);
    mocks.del.mockResolvedValue({ deleted: true });
  });

  it("opens a review starting on the version each prompt uses now", async () => {
    mocks.kept.mockResolvedValue({
      prompts: [kept("house-rules"), kept("notes", "local")],
      repository_only: [],
    });
    const { result } = await renderConfigured();
    let opened = "";
    await act(async () => {
      opened = await result.current.startRemoval();
    });
    expect(opened).toBe("review");
    expect(result.current.removal.choices).toEqual({
      "house-rules": "repository",
      notes: "local",
    });

    act(() => result.current.removal.setChoice("house-rules", "local"));
    let removed = false;
    await act(async () => {
      removed = await result.current.handleDelete();
    });
    expect(removed).toBe(true);
    expect(mocks.del).toHaveBeenCalledWith({
      choices: { "house-rules": "local", notes: "local" },
      delete_repository_only: [],
    });
    expect(result.current.removal.active).toBe(false);
    expect(result.current.config).toBeNull();
  });

  it("asks for the plain confirmation when no prompt keeps a local version", async () => {
    mocks.kept.mockResolvedValue({ prompts: [], repository_only: [] });
    const { result } = await renderConfigured();
    let opened = "";
    await act(async () => {
      opened = await result.current.startRemoval();
    });
    expect(opened).toBe("confirm");
    expect(result.current.removal.active).toBe(false);
    await act(async () => {
      await result.current.handleDelete();
    });
    expect(mocks.del).toHaveBeenCalledWith({ choices: {}, delete_repository_only: [] });
  });

  it("offers deleting the prompts that only came from the repository, off by default", async () => {
    mocks.kept.mockResolvedValue({ prompts: [], repository_only: ["principles", "release-notes"] });
    const { result } = await renderConfigured();
    let opened = "";
    await act(async () => {
      opened = await result.current.startRemoval();
    });
    expect(opened).toBe("review");
    expect(result.current.removal.active).toBe(true);
    expect(result.current.removal.repositoryOnly).toEqual(["principles", "release-notes"]);
    expect(result.current.removal.deleteRepositoryOnly).toBe(false);

    act(() => result.current.removal.setDeleteRepositoryOnly(true));
    await act(async () => {
      await result.current.handleDelete();
    });
    expect(mocks.del).toHaveBeenCalledWith({
      choices: {},
      delete_repository_only: ["principles", "release-notes"],
    });
    expect(result.current.removal.active).toBe(false);
    expect(result.current.removal.deleteRepositoryOnly).toBe(false);
  });

  it("closing the removal step resets the checkbox", async () => {
    mocks.kept.mockResolvedValue({ prompts: [], repository_only: ["principles"] });
    const { result } = await renderConfigured();
    await act(async () => {
      await result.current.startRemoval();
    });
    act(() => result.current.removal.setDeleteRepositoryOnly(true));
    act(() => result.current.removal.close());
    expect(result.current.removal.active).toBe(false);
    expect(result.current.removal.deleteRepositoryOnly).toBe(false);
  });

  it("removes nothing when the kept versions cannot be read", async () => {
    mocks.kept.mockRejectedValue(new Error("boom"));
    const { result } = await renderConfigured();
    let opened = "";
    await act(async () => {
      opened = await result.current.startRemoval();
    });
    expect(opened).toBe("failed");
    expect(mocks.del).not.toHaveBeenCalled();
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
