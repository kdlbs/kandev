import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { AgentRuntimeUpdateControl } from "./agent-runtime-update-control";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children, ...props }: { children?: ReactNode } & Record<string, unknown>) => (
    <div {...props}>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DrawerContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DrawerDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

const AGENT_NAME = "copilot-acp";
const PACKAGE_NAME = "@github/copilot";

function preview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return {
    update_mode: "pinned",
    agent_name: AGENT_NAME,
    package: PACKAGE_NAME,
    current_version: "1.0.104",
    default_version: "1.0.88",
    effective_version: "1.0.104",
    active_version: "1.0.104",
    target_version: "9.9.9",
    operation: "update",
    available_versions: [
      { version: "9.9.9", latest: true },
      { version: "1.0.104", latest: false },
      { version: "1.0.88", latest: false },
    ],
    command: ["npm", "exec"],
    command_string: "npm exec",
    ...overrides,
  };
}

function unsupportedPreview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return preview({
    copilot_plugin_compatibility: {
      runtime_version: "9.9.9",
      supported: false,
      supported_versions: ["1.0.88", "1.0.104", "1.0.105"],
      reason: "runtime_unsupported",
      selected_profile_count: 2,
    },
    ...overrides,
  });
}

function renderControl(onPreview: () => Promise<AgentUpdatePreview>) {
  const onUpdate = vi.fn().mockResolvedValue({
    job_id: "job-1",
    update_mode: "pinned",
    agent_name: AGENT_NAME,
    status: "queued",
    started_at: "2026-01-01T00:00:00.000Z",
  } satisfies AgentUpdateJob);
  render(
    <AgentRuntimeUpdateControl
      agentName={AGENT_NAME}
      displayName="Copilot"
      runtimeUpdate={{
        supported: true,
        update_mode: "pinned",
        package: PACKAGE_NAME,
        default_version: "1.0.88",
        effective_version: "1.0.104",
      }}
      onPreview={onPreview}
      onUpdate={onUpdate}
    />,
  );
  return onUpdate;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Copilot runtime plugin compatibility preview", () => {
  it("shows an unqualified target with mixed-profile impact before approval", async () => {
    const onUpdate = renderControl(() => Promise.resolve(unsupportedPreview()));
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));

    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.getAttribute("data-supported")).toBe("false");
    expect(notice.textContent).toContain("Copilot 9.9.9: plugin adapter unavailable.");
    expect(notice.textContent).toContain(
      "2 saved profiles have plugin selections that will block a new process.",
    );
    expect(notice.textContent).toContain("Qualified versions: 1.0.88, 1.0.104, 1.0.105");
    expect(notice.textContent).toContain(
      "Select a qualified runtime, or uncheck plugins and Save.",
    );
    const confirm = screen.getByTestId(`agent-update-confirm-${AGENT_NAME}`);
    expect(confirm.hasAttribute("disabled")).toBe(false);
    fireEvent.click(confirm);
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(AGENT_NAME, "9.9.9"));
  });

  it("shows unknown impact instead of zero when the profile read failed", async () => {
    renderControl(() =>
      Promise.resolve(
        unsupportedPreview({
          copilot_plugin_compatibility: {
            runtime_version: "9.9.9",
            supported: false,
            supported_versions: ["1.0.88", "1.0.104", "1.0.105"],
            reason: "runtime_unsupported",
            selected_profile_count: null,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain(
      "Saved profile impact is unknown because profiles could not be read.",
    );
    expect(notice.textContent).not.toContain("No saved profiles select plugins.");
  });

  it("keeps the target limitation visible when no saved profile selects plugins", async () => {
    renderControl(() =>
      Promise.resolve(
        unsupportedPreview({
          copilot_plugin_compatibility: {
            runtime_version: "9.9.9",
            supported: false,
            supported_versions: ["1.0.88", "1.0.104", "1.0.105"],
            reason: "runtime_unsupported",
            selected_profile_count: 0,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain("Copilot 9.9.9: plugin adapter unavailable.");
    expect(notice.textContent).toContain(
      "No saved profiles select plugins. The runtime's plugin limitation still applies.",
    );
  });

  it("reports qualification without promising every selected plugin can load", async () => {
    renderControl(() =>
      Promise.resolve(
        preview({
          copilot_plugin_compatibility: {
            runtime_version: "1.0.104",
            supported: true,
            supported_versions: ["1.0.88", "1.0.104", "1.0.105"],
            selected_profile_count: 2,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.getAttribute("data-supported")).toBe("true");
    expect(notice.textContent).toContain(
      "Copilot 1.0.104 supports profile plugin preparation. Selected plugins still depend on their own installation and native policy.",
    );
    expect(notice.textContent).toContain("Qualified versions: 1.0.88, 1.0.104, 1.0.105");
  });

  it("uses an unavailable-compatibility fallback for an older Copilot server", async () => {
    renderControl(() => Promise.resolve(preview()));
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain(
      "Plugin compatibility information is unavailable from this server. Check this runtime's plugin support before updating.",
    );
  });
});
