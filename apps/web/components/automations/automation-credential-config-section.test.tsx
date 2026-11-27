import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormState } from "./automation-payload";

vi.mock("./config-section", () => ({
  ConfigSection: (props: {
    onAgentProfileChange: (profileId: string) => void;
    onPersonalCredentialBindingsChange: (bindings: FormState["personalCredentialBindings"]) => void;
    credentialChoicesEligible: boolean;
  }) => (
    <div>
      <button
        data-testid="change-agent-profile"
        onClick={() => props.onAgentProfileChange("agent-2")}
      />
      <button
        data-testid="change-personal-binding"
        onClick={() => props.onPersonalCredentialBindingsChange([])}
      />
      <span data-testid="credential-eligibility">{String(props.credentialChoicesEligible)}</span>
    </div>
  ),
}));

import { AutomationCredentialConfig } from "./automation-credential-config-section";

const form: FormState = {
  name: "Credential automation",
  description: "",
  workflowId: "workflow-1",
  workflowStepId: "",
  agentProfileId: "agent-1",
  executorProfileId: "executor-1",
  taskMode: "automation_run",
  repositoryMode: "none",
  repositorySelections: [],
  prompt: "Run",
  taskTitleTemplate: "",
  enabled: false,
  maxConcurrentRuns: 1,
  continuationPolicy: "reuse_thread",
  followUpPrompt: "",
  prContinuationStepId: "",
  prMergedAction: "keep",
  prClosedAction: "keep",
  retryPolicy: {
    mode: "disabled",
    max_retries: "0",
    delay_seconds: "0",
    backoff: "fixed",
    history_mode: "attempts",
  },
  pluginTools: [],
  personalCredentialBindings: [
    { profile_id: "agent-1", key: "SERVICE_KEY", secret_id: "secret-1" },
  ],
};

afterEach(() => {
  cleanup();
});

describe("AutomationCredentialConfig", () => {
  it("does not mutate personal choices through either selector when the draft is ineligible", () => {
    const updateField = vi.fn();
    render(
      <AutomationCredentialConfig
        form={form}
        triggers={[{ id: "trigger-1", type: "scheduled" } as never]}
        workspaceId="workspace-1"
        automation={null}
        onAutomationChange={() => {}}
        updateField={updateField}
        credentialError={null}
        dirtyFields={{
          agentProfileId: false,
          executorProfileId: false,
          repositorySelections: false,
        }}
      />,
    );

    expect(screen.getByTestId("credential-eligibility").textContent).toBe("false");
    fireEvent.click(screen.getByTestId("change-personal-binding"));
    fireEvent.click(screen.getByTestId("change-agent-profile"));

    expect(updateField).toHaveBeenCalledWith("agentProfileId", "agent-2");
    expect(updateField).not.toHaveBeenCalledWith("personalCredentialBindings", expect.anything());
  });
});
