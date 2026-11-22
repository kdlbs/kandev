import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type {
  PromptRemovalReview,
  PromptSyncController,
} from "@/hooks/domains/settings/use-prompt-sync";
import type { PromptSyncConfig, PromptSyncConflict } from "@/lib/types/prompt-sync";
import type { PromptConflictReview } from "@/hooks/domains/settings/use-prompt-conflict-review";

const mocks = vi.hoisted(() => ({
  controller: null as unknown as PromptSyncController,
}));

vi.mock("@/hooks/domains/settings/use-prompt-sync", () => ({
  usePromptSync: () => mocks.controller,
}));
const storeState = vi.hoisted(() => ({
  prompts: [] as Array<{ id: string; name: string; source?: string; conflict?: boolean }>,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "ws-1", items: [{ id: "ws-1", name: "Core" }] },
      prompts: { items: storeState.prompts },
    }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

import { PromptSyncSection } from "./prompt-sync-section";

const config: PromptSyncConfig = {
  workspace_id: "ws-1",
  provider: "gitlab",
  repo_owner: "",
  repo_name: "",
  project_path: "acme/eng",
  branch: "main",
  path: ".kandev/prompts",
  interval_seconds: 300,
  poll_enabled: true,
  last_ok: true,
  last_synced_at: "2026-11-19T10:00:00Z",
  last_warnings: ['prompt "review" was kept'],
  created_at: "2026-11-19T10:00:00Z",
  updated_at: "2026-11-19T10:00:00Z",
};

const OPEN_TEST_ID = "prompt-sync-open";
const SAVE_TEST_ID = "prompt-sync-save";

function makeController(overrides: Partial<PromptSyncController> = {}): PromptSyncController {
  return {
    config: null,
    form: {
      workspace_id: "ws-1",
      provider: "github",
      github_host: "github.com",
      repo_owner: "",
      repo_name: "",
      project_path: "",
      branch: "main",
      path: ".kandev/prompts",
      interval_seconds: 300,
      poll_enabled: true,
    },
    url: "",
    urlInvalid: false,
    loading: false,
    saving: false,
    syncing: false,
    update: vi.fn(),
    setUrlInput: vi.fn(),
    setProvider: vi.fn(),
    reset: vi.fn(),
    handleSave: vi.fn().mockResolvedValue(true),
    handleDelete: vi.fn().mockResolvedValue(true),
    handleSyncNow: vi.fn().mockResolvedValue(undefined),
    review: makeReview(),
    removal: makeRemoval(),
    startRemoval: vi.fn().mockResolvedValue("confirm"),
    backToFields: vi.fn(),
    ...overrides,
  };
}

const conflict: PromptSyncConflict = {
  name: "principles",
  path: ".kandev/prompts/principles.md",
  local_content: "Mine",
  repository_content: "Repo",
};

function makeReview(conflicts: PromptSyncConflict[] = []): PromptConflictReview {
  return {
    active: conflicts.length > 0,
    conflicts,
    choices: Object.fromEntries(conflicts.map((item) => [item.name, "local" as const])),
    open: vi.fn(),
    setChoice: vi.fn(),
    close: vi.fn(),
  };
}

function makeRemoval(
  conflicts: PromptSyncConflict[] = [],
  repositoryOnly: string[] = [],
): PromptRemovalReview {
  return {
    ...makeReview(conflicts),
    active: conflicts.length > 0 || repositoryOnly.length > 0,
    repositoryOnly,
    setRepositoryOnly: vi.fn(),
    deleteRepositoryOnly: false,
    setDeleteRepositoryOnly: vi.fn(),
  };
}

function renderSection() {
  return render(
    <TooltipProvider>
      <PromptSyncSection />
    </TooltipProvider>,
  );
}

beforeEach(() => {
  mocks.controller = makeController();
});
afterEach(cleanup);

// @covers AC-INTEGRATIONS-PROMPT-SYNC-001.1 AC-INTEGRATIONS-PROMPT-SYNC-001.3 AC-INTEGRATIONS-PROMPT-SYNC-003.7
describe("PromptSyncSection", () => {
  it("offers setup when unconfigured and requires a repository before saving", () => {
    renderSection();
    expect(screen.queryByTestId("prompt-sync-status")).toBeNull();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(mocks.controller.reset).toHaveBeenCalledWith(null);
    expect(screen.getByTestId("prompt-sync-dialog")).toBeTruthy();
    expect(screen.getByTestId("prompt-sync-workspace-select")).toBeTruthy();
    expect((screen.getByTestId(SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId("prompt-sync-remove")).toBeNull();
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-004.3
  it("draws one outline: no group card around the empty or configured section", () => {
    const view = renderSection();
    const section = screen.getByTestId("prompt-sync-section");
    expect(section.querySelector("[data-settings-group-card]")).toBeNull();
    mocks.controller = makeController({ config });
    view.rerender(
      <TooltipProvider>
        <PromptSyncSection />
      </TooltipProvider>,
    );
    const configured = screen.getByTestId("prompt-sync-section");
    expect(configured.querySelector("[data-settings-group-card]")).toBeNull();
    expect(configured.querySelector('[data-testid="prompt-sync-status"]')).not.toBeNull();
  });

  it("saves a complete form and closes the dialog", async () => {
    mocks.controller = makeController({
      form: { ...makeController().form, repo_owner: "acme", repo_name: "eng" },
    });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    const save = screen.getByTestId(SAVE_TEST_ID) as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(mocks.controller.handleSave).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByTestId("prompt-sync-dialog")).toBeNull());
  });

  it("shows status, warnings, and Sync now when configured", () => {
    mocks.controller = makeController({ config });
    renderSection();
    expect(screen.getByTestId("prompt-sync-status").getAttribute("data-state")).toBe("ok");
    expect(screen.getByTestId("prompt-sync-warnings").textContent).toContain("review");
    fireEvent.click(screen.getByTestId("prompt-sync-now"));
    expect(mocks.controller.handleSyncNow).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.getByTestId("prompt-sync-remove")).toBeTruthy();
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.5
describe("PromptSyncSection overrides", () => {
  it("lists prompts overridden locally apart from warnings", () => {
    storeState.prompts = [
      { id: "1", name: "hook", source: "override" },
      { id: "2", name: "rule", source: "sync" },
    ];
    mocks.controller = makeController({ config });
    renderSection();
    const list = screen.getByTestId("prompt-sync-overridden");
    expect(list.textContent).toContain("@hook");
    expect(list.textContent).not.toContain("@rule");
    storeState.prompts = [];
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-008.2
describe("PromptSyncSection conflict review before saving", () => {
  it("replaces the fields with the review step and goes back without saving", () => {
    mocks.controller = makeController({ review: makeReview([conflict]) });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.queryByTestId("prompt-sync-workspace-select")).toBeNull();
    const item = screen.getByTestId("prompt-conflict-item");
    expect(item.getAttribute("data-prompt-name")).toBe("principles");
    fireEvent.click(screen.getByTestId("prompt-conflict-compare"));
    expect(screen.getByTestId("prompt-conflict-local-content").textContent).toBe("Mine");
    expect(screen.getByTestId("prompt-conflict-repository-content").textContent).toBe("Repo");
    fireEvent.click(screen.getByTestId("prompt-sync-review-back"));
    expect(mocks.controller.backToFields).toHaveBeenCalled();
    expect(mocks.controller.handleSave).not.toHaveBeenCalled();
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-008.2
describe("PromptConflictReviewList", () => {
  it("explains the two versions once and gives each prompt one switch row", () => {
    const second = { ...conflict, name: "notes", path: ".kandev/prompts/notes.md" };
    mocks.controller = makeController({ review: makeReview([conflict, second]) });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.getAllByTestId("prompt-conflict-legend")).toHaveLength(1);
    expect(screen.getAllByText(/stays in use as a local override/)).toHaveLength(1);
    const rows = screen.getAllByTestId("prompt-conflict-item");
    expect(rows.map((row) => row.getAttribute("data-prompt-name"))).toEqual([
      "principles",
      "notes",
    ]);
    expect(screen.getAllByTestId("prompt-conflict-switch")).toHaveLength(2);
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-009.5
describe("PromptSyncSection removal with kept local versions", () => {
  it("asks for the plain confirmation when no prompt keeps a local version", async () => {
    mocks.controller = makeController({ config });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    fireEvent.click(screen.getByTestId("prompt-sync-remove"));
    await waitFor(() => expect(screen.getByTestId("prompt-sync-remove-confirmation")).toBeTruthy());
    expect(mocks.controller.startRemoval).toHaveBeenCalled();
  });

  it("lists prompts with both versions as switch rows and removes from that step", async () => {
    mocks.controller = makeController({
      config,
      removal: { ...makeRemoval([conflict]), choices: { principles: "repository" } },
    });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.queryByTestId("prompt-sync-workspace-select")).toBeNull();
    expect(screen.getByTestId("prompt-conflict-legend").textContent).toContain(
      "keeps the repository version",
    );
    const toggle = screen.getByRole("switch", {
      name: "Use the repository version of @principles",
    });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(toggle);
    expect(mocks.controller.removal.setChoice).toHaveBeenCalledWith("principles", "local");
    fireEvent.click(screen.getByTestId("prompt-sync-removal-back"));
    expect(mocks.controller.removal.close).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("prompt-sync-removal-confirm"));
    await waitFor(() => expect(mocks.controller.handleDelete).toHaveBeenCalled());
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-009.6
  it("offers deleting the prompts that only came from the repository", async () => {
    mocks.controller = makeController({
      config,
      removal: makeRemoval([], ["release-notes", "principles"]),
    });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.queryByTestId("prompt-conflict-legend")).toBeNull();
    expect(screen.getByTestId("prompt-removal-repository-only-names").textContent).toBe(
      "@release-notes, @principles",
    );
    const checkbox = screen.getByRole("checkbox", {
      name: "Also delete 2 prompts that only came from the repository",
    });
    expect(checkbox.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(checkbox);
    expect(mocks.controller.removal.setDeleteRepositoryOnly).toHaveBeenCalledWith(true);
  });
});
