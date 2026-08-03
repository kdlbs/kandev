import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { SlackConfig } from "@/lib/types/slack";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  setConfig: vi.fn(),
  deleteConfig: vi.fn(),
  testConnection: vi.fn(),
  listUtilityAgents: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 100_000,
}));
vi.mock("@/hooks/domains/slack/use-slack-enabled", () => ({
  useSlackEnabled: () => ({ enabled: true, setEnabled: vi.fn() }),
}));
vi.mock("@/lib/api/domains/slack-api", () => ({
  getSlackConfig: mocks.getConfig,
  setSlackConfig: mocks.setConfig,
  deleteSlackConfig: mocks.deleteConfig,
  testSlackConnection: mocks.testConnection,
}));
vi.mock("@/lib/api/domains/utility-api", () => ({
  listUtilityAgents: mocks.listUtilityAgents,
}));

import { SlackConnectionSection } from "@/components/slack/slack-settings";

const CONFIG: SlackConfig = {
  authMethod: "cookie",
  utilityAgentId: "agent-1",
  commandPrefix: "!kandev",
  pollIntervalSeconds: 30,
  hasToken: true,
  hasCookie: true,
  lastOk: true,
  createdAt: "2026-08-01T00:00:00Z",
  updatedAt: "2026-08-01T00:00:00Z",
};

const AGENTS = [
  { id: "agent-1", name: "Triage", model: "claude-opus-5", enabled: true, agent_id: "claude" },
  { id: "agent-2", name: "Builtin", builtin: true },
  { id: "agent-3", name: "Paused", enabled: false, agent_id: "claude" },
] as unknown as UtilityAgent[];

function renderSection() {
  return render(
    <StateProvider>
      <SettingsSaveProvider>
        <SlackConnectionSection workspaceId="workspace-a" />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getConfig.mockResolvedValue(CONFIG);
  mocks.listUtilityAgents.mockResolvedValue({ agents: AGENTS });
});

afterEach(cleanup);

describe("SlackConnectionSection", () => {
  // Each block below is a `<Trans>` whose `<n>` indices address the JSX children
  // positionally, so a reflow of those children silently reassembles the
  // sentence into fragments. Assert the whole reconstructed paragraph.
  it("renders the credential and prompt help with their verbatim tokens", async () => {
    renderSection();
    await waitFor(() => expect(mocks.getConfig).toHaveBeenCalled());

    expect(
      screen.getByText(
        "Open Slack in your browser, copy the value of the `d` cookie and the `xoxc-` token from local storage. Both are required.",
      ),
    ).toBeTruthy();

    const promptHelp = screen.getByText(/Custom prompts can reference/);
    expect(promptHelp.textContent).toBe(
      "Custom prompts can reference {{SlackInstruction}}, {{SlackThread}}, {{SlackPermalink}}, " +
        "{{SlackUser}}, {{SlackChannelID}}, {{SlackTS}}. When at least one is used, your template " +
        "owns the full prompt; otherwise the default Slack-triage system prompt is prepended " +
        "automatically.",
    );

    const prefixHelp = screen.getByText(/Messages you write in Slack/);
    expect(prefixHelp.textContent).toBe(
      "Messages you write in Slack starting with this prefix become Kandev tasks. Default: " +
        "!kandev <instruction>.",
    );

    const pollHelp = screen.getByText(/How often Slack is checked/);
    expect(pollHelp.textContent).toBe(
      "How often Slack is checked for new !kandev messages. Lower = more responsive, higher = " +
        "fewer Slack API calls. Range: 5–600s. Default: 30s.",
    );

    // `<0>` is the <strong> lead-in, so getByText matches it rather than the
    // paragraph — assert on the parent to cover the whole reassembled sentence.
    const warningLead = screen.getByText("Browser session auth (unsupported):");
    expect(warningLead.tagName).toBe("STRONG");
    expect(warningLead.parentElement?.textContent).toBe(
      "Browser session auth (unsupported): Slack rotates session cookies often, so you may need " +
        "to reconnect when authentication expires. Bot installs and user OAuth are on the roadmap.",
    );
  });

  it("labels utility agents from the record rather than concatenated copy", async () => {
    renderSection();
    await waitFor(() => expect(mocks.listUtilityAgents).toHaveBeenCalled());

    // The trigger renders the selected agent's label.
    await waitFor(() =>
      expect(screen.getByText("Triage (claude-opus-5)", { exact: false })).toBeTruthy(),
    );
  });
});
