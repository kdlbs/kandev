import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { PromptSyncController } from "@/hooks/domains/settings/use-prompt-sync";
import type { PromptSyncConfig } from "@/lib/types/prompt-sync";

const mocks = vi.hoisted(() => ({ controller: null as unknown as PromptSyncController }));

vi.mock("@/hooks/domains/settings/use-prompt-sync", () => ({
  usePromptSync: () => mocks.controller,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ workspaces: { activeId: "ws-1", items: [{ id: "ws-1", name: "Core" }] } }),
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
    ...overrides,
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
