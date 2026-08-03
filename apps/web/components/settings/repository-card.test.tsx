import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RepositoryCard } from "./repository-card";
import { SettingsSaveProvider } from "./settings-save-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Repository, RepositoryScript } from "@/lib/types/http";

afterEach(cleanup);

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

/**
 * The collapsed card's whole summary is built by `buildRepoPreviewData` /
 * `buildRepoScriptsSummary` — plain functions holding no JSX, which
 * `i18next/no-literal-string` never inspects. The script count also used to be
 * inflected with an English `s` at the call site. Neither shape fails loudly, so
 * these assertions pin the rendered text.
 */
function repository(overrides: Partial<RepositoryWithScripts> = {}): RepositoryWithScripts {
  return {
    id: "repo-1",
    workspace_id: "ws-1",
    name: "kandev",
    source_type: "local",
    local_path: "/home/dev/kandev",
    provider: "",
    provider_repo_id: "",
    provider_host: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    worktree_branch_template: "feature/{title}-{suffix}",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    scripts: [],
    ...overrides,
  } as RepositoryWithScripts;
}

function script(id: string): RepositoryScript {
  return {
    id,
    repository_id: "repo-1",
    name: id,
    command: "echo hi",
    position: 0,
    created_at: "",
    updated_at: "",
  } as RepositoryScript;
}

function renderCard(repo: RepositoryWithScripts) {
  return render(
    <ToastProvider>
      <SettingsSaveProvider>
        <RepositoryCard
          repository={repo}
          savedRepository={repo}
          isRepositoryDirty={false}
          areScriptsDirty={false}
          onUpdate={vi.fn()}
          onAddScript={vi.fn()}
          onUpdateScript={vi.fn()}
          onDeleteScript={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
        />
      </SettingsSaveProvider>
    </ToastProvider>,
  );
}

describe("RepositoryCard preview", () => {
  it("uses the singular form for exactly one custom script", () => {
    renderCard(repository({ scripts: [script("a")] }));

    expect(screen.getByText("1 custom script")).toBeTruthy();
  });

  it("uses the plural form for more than one custom script", () => {
    renderCard(repository({ scripts: [script("a"), script("b")] }));

    expect(screen.getByText("2 custom scripts")).toBeTruthy();
  });

  it("labels a local repository and shows its path verbatim", () => {
    renderCard(repository());

    expect(screen.getByText("Local")).toBeTruthy();
    expect(screen.getByText("/home/dev/kandev")).toBeTruthy();
  });

  it("falls back to translated copy when a local repository has no path", () => {
    renderCard(repository({ local_path: "" }));

    expect(screen.getByText("Local path not set")).toBeTruthy();
  });

  it("shows the owner/name slug for a remote repository", () => {
    renderCard(
      repository({
        source_type: "remote",
        provider_owner: "kdlbs",
        provider_name: "kandev",
      }),
    );

    expect(screen.getByText("Remote")).toBeTruthy();
    expect(screen.getByText("kdlbs/kandev")).toBeTruthy();
  });

  it("falls back to translated copy when a remote repository has no slug", () => {
    renderCard(repository({ source_type: "remote" }));

    expect(screen.getByText("Remote repository")).toBeTruthy();
  });

  it("names an unnamed repository", () => {
    renderCard(repository({ name: "" }));

    expect(screen.getByText("Untitled repository")).toBeTruthy();
  });

  it("lists the three built-in script chips only when their scripts are set", () => {
    renderCard(
      repository({
        setup_script: "npm ci",
        cleanup_script: "rm -rf tmp",
        dev_script: "npm run dev",
      }),
    );

    expect(screen.getByText("build script")).toBeTruthy();
    expect(screen.getByText("cleanup script")).toBeTruthy();
    expect(screen.getByText("dev script")).toBeTruthy();
    expect(screen.queryByText("No custom scripts")).toBeNull();
  });
});
