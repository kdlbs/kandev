import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StatusMessage } from "./status-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

afterEach(cleanup);

const STATUS_CREATED_AT = "2026-08-15T00:00:00Z";

function modelSelectionWarningMessage(): Message {
  return {
    id: "status-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "status",
    created_at: STATUS_CREATED_AT,
    metadata: {
      variant: "warning",
      kind: "model_selection_warning",
      reason: "requested_not_advertised",
      requested_model: "claude-sonnet-4",
      effective_model: "claude-haiku-4",
      agent_id: "claude-acp",
      executor_type: "ssh",
      executor_profile_id: "executor-1",
      remediation: ["executor_credentials", "copied_agent_configuration", "agent_version"],
    },
  };
}

describe("StatusMessage model selection warnings", () => {
  it("renders the persisted decision context and remediation guidance", () => {
    render(<StatusMessage comment={modelSelectionWarningMessage()} />);

    expect(screen.getByText("The executor could not use the saved model selection.")).toBeTruthy();
    expect(screen.getByText("claude-sonnet-4")).toBeTruthy();
    expect(screen.getByText("claude-haiku-4")).toBeTruthy();
    expect(screen.getByText("claude-acp")).toBeTruthy();
    expect(screen.getByText("executor-1")).toBeTruthy();
    expect(screen.getByText("Check executor credentials.")).toBeTruthy();
    expect(screen.getByText("Check the copied agent configuration.")).toBeTruthy();
    expect(screen.getByText("Check the agent version in the executor.")).toBeTruthy();
    expect(screen.getByText("The saved model was not advertised by the executor.")).toBeTruthy();
  });

  it("renders a retired variation reason from a persisted legacy warning", () => {
    const comment = modelSelectionWarningMessage();
    comment.metadata = {
      ...comment.metadata,
      reason: "unique_variation_applied",
      effective_model: "opus[1m]",
      fallback_model: undefined,
    };

    render(<StatusMessage comment={comment} />);

    expect(
      screen.getByText("The executor applied the only advertised variation of the saved model."),
    ).toBeTruthy();
    expect(screen.queryByText(/fallback model/i)).toBeNull();
  });
});

describe("StatusMessage branch replacement warnings", () => {
  it("states that conversation history continued but lost code did not", () => {
    const comment: Message = {
      id: "status-branch-1",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: "branch_recreated",
      type: "status",
      created_at: STATUS_CREATED_AT,
      metadata: {
        variant: "warning",
        kind: "branch_recreated",
        original_branch: "feature/lost",
        new_branch: "kandev/task-recovery-1",
        base_branch: "main",
      },
    };

    render(<StatusMessage comment={comment} />);

    expect(screen.getByTestId("branch-recreated-warning")).toBeTruthy();
    expect(screen.getByText("feature/lost")).toBeTruthy();
    expect(screen.getByText("kandev/task-recovery-1")).toBeTruthy();
    expect(screen.getByText("main")).toBeTruthy();
    expect(screen.getByText(/conversation history continues/i)).toBeTruthy();
    expect(screen.getByText(/code changes.*not recovered/i)).toBeTruthy();
  });
});

describe("StatusMessage explicit resume recovery", () => {
  it("uses only the notice's confirmed provider model and explains preserved conversation", () => {
    const backendContent =
      "Session resumed with the provider's current settings. Saved mode and model selections were kept for future launches.";
    const comment: Message = {
      id: "status-resume-recovery-1",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: backendContent,
      type: "status",
      created_at: STATUS_CREATED_AT,
      metadata: {
        variant: "resume_settings_provider_restored",
        settings_policy: "provider_restored",
        attempt_id: "attempt-1",
        skipped_settings: ["mode", "model"],
        effective_mode_known: true,
        effective_mode_id: "safe",
        effective_model_known: true,
        effective_model_id: "gpt-5.6-sol",
        effective_model_name: "Gemini 3.7 Flash",
      },
    };

    render(<StatusMessage comment={comment} />);

    expect(
      screen.getByText(
        "Session resumed with Gemini 3.7 Flash. Your previous conversation was preserved.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Saved mode and model selections remain unchanged for future launches. This attempt skipped both overrides. The restored permission mode may differ.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(backendContent)).toBeNull();
  });

  it("uses the confirmed model ID when the attempt snapshot has no catalog label", () => {
    const comment = providerRestoredSuccessMessage({
      effective_model_known: true,
      effective_model_id: "provider-model-id",
    });
    render(<StatusMessage comment={comment} />);
    expect(
      screen.getByText(
        "Session resumed with provider-model-id. Your previous conversation was preserved.",
      ),
    ).toBeTruthy();
  });
});

const PRIVATE_TOKEN = "synthetic-private-value";
const PRIVATE_PASSWORD = "synthetic-password";
const PRIVATE_PATH = "synthetic-private-model";
const CONTROL_MARKER = "synthetic-control";
const PRIVATE_MODEL_LABEL = "Synthetic Private Model";

describe("StatusMessage legacy provider model sanitization", () => {
  it.each([
    `token=${PRIVATE_TOKEN}`,
    `https://alice:${PRIVATE_PASSWORD}@private.example/models/key`,
    `/home/alice/${PRIVATE_PATH}`,
    `Gemini\u001b[31m${CONTROL_MARKER}`,
  ])("falls back to the confirmed ID for unsafe legacy catalog label %s", (modelName) => {
    render(
      <StatusMessage
        comment={providerRestoredSuccessMessage({
          effective_model_known: true,
          effective_model_id: "vendor/model-5",
          effective_model_name: modelName,
        })}
      />,
    );

    expect(
      screen.getByText(
        "Session resumed with vendor/model-5. Your previous conversation was preserved.",
      ),
    ).toBeTruthy();
    expectNoProviderSecretsInDOM(PRIVATE_TOKEN, PRIVATE_PASSWORD, PRIVATE_PATH, CONTROL_MARKER);
  });

  it.each([
    `token=${PRIVATE_TOKEN}`,
    `https://alice:${PRIVATE_PASSWORD}@private.example/models/key`,
    `/home/alice/${PRIVATE_PATH}`,
    "vendor/model-5\u001b",
  ])("uses unknown-model success for unsafe legacy model ID %s", (modelID) => {
    render(
      <StatusMessage
        comment={providerRestoredSuccessMessage({
          effective_model_known: true,
          effective_model_id: modelID,
          effective_model_name: PRIVATE_MODEL_LABEL,
        })}
      />,
    );

    expect(
      screen.getByText("Session resumed. Your previous conversation was preserved."),
    ).toBeTruthy();
    expectNoProviderSecretsInDOM(
      PRIVATE_TOKEN,
      PRIVATE_PASSWORD,
      PRIVATE_PATH,
      PRIVATE_MODEL_LABEL,
    );
  });

  it("does not claim a model when the attempt snapshot says the effective model is unknown", () => {
    const comment = providerRestoredSuccessMessage({
      effective_model_known: false,
      effective_model_id: "stale-selector-value",
      effective_model_name: "Stale catalog label",
    });
    render(<StatusMessage comment={comment} />);
    expect(
      screen.getByText("Session resumed. Your previous conversation was preserved."),
    ).toBeTruthy();
    expect(screen.queryByText(/stale-selector-value|Stale catalog label/)).toBeNull();
  });
});

function expectNoProviderSecretsInDOM(...values: string[]) {
  for (const value of values) {
    expect(document.body.textContent).not.toContain(value);
    expect(document.body.innerHTML).not.toContain(value);
  }
}

function providerRestoredSuccessMessage(selectorMetadata: Record<string, unknown>): Message {
  return {
    id: "status-resume-recovery-2",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "Provider-restored resume succeeded.",
    type: "status",
    created_at: STATUS_CREATED_AT,
    metadata: {
      variant: "resume_settings_provider_restored",
      settings_policy: "provider_restored",
      attempt_id: "attempt-2",
      skipped_settings: ["mode", "model"],
      ...selectorMetadata,
    },
  };
}
