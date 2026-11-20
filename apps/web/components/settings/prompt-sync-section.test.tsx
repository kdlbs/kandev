import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { PromptSyncController } from "@/hooks/domains/settings/use-prompt-sync";
import type { PromptSyncConfig, PromptSyncConflict } from "@/lib/types/prompt-sync";
import type { PromptConflictResolver } from "@/hooks/domains/settings/use-prompt-conflict-resolver";
import type { PromptConflictReview } from "@/hooks/domains/settings/use-prompt-conflict-review";

const mocks = vi.hoisted(() => ({
  controller: null as unknown as PromptSyncController,
  resolver: null as unknown as PromptConflictResolver,
}));

vi.mock("@/hooks/domains/settings/use-prompt-sync", () => ({
  usePromptSync: () => mocks.controller,
}));
vi.mock("@/hooks/domains/settings/use-prompt-conflict-resolver", () => ({
  usePromptConflictResolver: () => mocks.resolver,
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
    removal: makeReview(),
    startRemoval: vi.fn().mockResolvedValue("confirm"),
    backToFields: vi.fn(),
    acceptConfig: vi.fn(),
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

function makeResolver(conflicts: PromptSyncConflict[] = []): PromptConflictResolver {
  return {
    review: makeReview(conflicts),
    busy: false,
    load: vi.fn().mockResolvedValue(true),
    apply: vi.fn().mockResolvedValue(true),
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
  mocks.resolver = makeResolver();
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

// @covers AC-INTEGRATIONS-PROMPT-SYNC-008.5 AC-INTEGRATIONS-PROMPT-SYNC-008.6
describe("PromptSyncSection unresolved conflicts", () => {
  afterEach(() => {
    storeState.prompts = [];
  });

  it("shows a banner whose Review opens the review dialog and applies the choices", async () => {
    storeState.prompts = [{ id: "1", name: "principles", source: "override", conflict: true }];
    mocks.controller = makeController({ config });
    mocks.resolver = makeResolver([conflict]);
    renderSection();
    expect(screen.getByTestId("prompt-conflict-banner").textContent).toContain("1");
    fireEvent.click(screen.getByTestId("prompt-conflict-banner-review"));
    await waitFor(() => expect(screen.getByTestId("prompt-conflict-dialog")).toBeTruthy());
    expect(mocks.resolver.load).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("prompt-conflict-apply"));
    expect(mocks.resolver.apply).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByTestId("prompt-conflict-dialog")).toBeNull());
  });

  it("shows no banner without conflicts", () => {
    storeState.prompts = [{ id: "1", name: "hook", source: "override" }];
    mocks.controller = makeController({ config });
    renderSection();
    expect(screen.queryByTestId("prompt-conflict-banner")).toBeNull();
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

  it("shows the version choices, repository first, and removes from that step", async () => {
    mocks.controller = makeController({
      config,
      removal: { ...makeReview([conflict]), choices: { principles: "repository" } },
    });
    renderSection();
    fireEvent.click(screen.getByTestId(OPEN_TEST_ID));
    expect(screen.queryByTestId("prompt-sync-workspace-select")).toBeNull();
    const choices = screen.getAllByTestId(/^prompt-conflict-choice-/);
    expect(choices[0]!.getAttribute("data-testid")).toBe("prompt-conflict-choice-repository");
    expect(choices[1]!.textContent).toContain("Restore my version");
    fireEvent.click(screen.getByTestId("prompt-sync-removal-back"));
    expect(mocks.controller.removal.close).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("prompt-sync-removal-confirm"));
    await waitFor(() => expect(mocks.controller.handleDelete).toHaveBeenCalled());
  });
});
