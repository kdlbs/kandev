import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PromptConflictResolver } from "@/hooks/domains/settings/use-prompt-conflict-resolver";
import type { PromptSyncConflict } from "@/lib/types/prompt-sync";

const state = vi.hoisted(() => ({
  prompts: [] as Array<{ id: string; name: string; source?: string; conflict?: boolean }>,
  resolver: null as unknown as PromptConflictResolver,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ prompts: { items: state.prompts } }),
}));
vi.mock("@/hooks/domains/settings/use-prompt-conflict-resolver", () => ({
  usePromptConflictResolver: () => state.resolver,
}));

import { PromptConflictNotice } from "./prompt-conflict-review-dialog";

const conflict: PromptSyncConflict = {
  name: "house-rules",
  path: ".kandev/prompts/house-rules.md",
  local_content: "Mine",
  repository_content: "Repo",
};

function makeResolver(): PromptConflictResolver {
  return {
    review: {
      active: true,
      conflicts: [conflict],
      choices: { "house-rules": "local" },
      open: vi.fn(),
      setChoice: vi.fn(),
      close: vi.fn(),
    },
    busy: false,
    load: vi.fn().mockResolvedValue(true),
    apply: vi.fn().mockResolvedValue(true),
  };
}

afterEach(cleanup);

// @covers AC-INTEGRATIONS-PROMPT-SYNC-008.5 AC-INTEGRATIONS-PROMPT-SYNC-008.6
describe("PromptConflictNotice", () => {
  it("names the conflicting prompts and resolves them from the review dialog", async () => {
    state.prompts = [
      { id: "1", name: "house-rules", source: "override", conflict: true },
      { id: "2", name: "late-rule", source: "override", conflict: true },
      { id: "3", name: "hook", source: "override" },
    ];
    state.resolver = makeResolver();
    render(<PromptConflictNotice />);
    const banner = screen.getByTestId("prompt-conflict-banner");
    expect(banner.getAttribute("role")).toBe("alert");
    expect(banner.textContent).toContain("2 prompt conflicts");
    expect(banner.textContent).toContain("@house-rules, @late-rule");
    expect(banner.textContent).not.toContain("@hook");

    fireEvent.click(screen.getByRole("button", { name: "Review conflicts" }));
    await waitFor(() => expect(screen.getByTestId("prompt-conflict-dialog")).toBeTruthy());
    fireEvent.click(screen.getByTestId("prompt-conflict-apply"));
    expect(state.resolver.apply).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByTestId("prompt-conflict-dialog")).toBeNull());
  });

  it("renders nothing without conflicts", () => {
    state.prompts = [{ id: "3", name: "hook", source: "override" }];
    state.resolver = makeResolver();
    render(<PromptConflictNotice />);
    expect(screen.queryByTestId("prompt-conflict-banner")).toBeNull();
  });
});
