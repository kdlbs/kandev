import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { StatusMessage } from "./status-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

afterEach(cleanup);
afterEach(() => vi.useRealTimers());

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

describe("StatusMessage Copilot interruption notices", () => {
  it("shows a clarification summary and opens the original warning as plain text", () => {
    const comment: Message = {
      id: "copilot-notice-1",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: "**Warning:** verify the current state.",
      type: "status",
      created_at: STATUS_CREATED_AT,
      metadata: { copilot_interruption_notice: "clarification" },
    };
    render(<StatusMessage comment={comment} />);

    expect(
      screen.getByText("The pending question was interrupted. You can continue the conversation."),
    ).toBeTruthy();
    expect(screen.queryByText(comment.content)).toBeNull();
    const details = screen.getByRole("button", { name: "Details" });
    expect(details.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(details);

    expect(screen.getByTestId("copilot-interruption-details").textContent).toBe(comment.content);
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByRole("button", { name: "Hide details" }).getAttribute("aria-expanded")).toBe(
      "true",
    );
  });

  it("shows the generic tool summary and provides a coarse-pointer target", () => {
    const comment: Message = {
      id: "copilot-notice-2",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: "Warning: verify the tool effects.",
      type: "status",
      created_at: STATUS_CREATED_AT,
      metadata: { copilot_interruption_notice: "tool" },
    };

    render(<StatusMessage comment={comment} />);

    expect(
      screen.getByText("A tool call was interrupted. Check its effects before retrying."),
    ).toBeTruthy();
    const details = screen.getByRole("button", { name: "Details" });
    expect(details.className).toContain("[@media(pointer:coarse)]:min-h-11");
    fireEvent.click(details);
    expect(screen.getByTestId("copilot-interruption-details").textContent).toBe(comment.content);
  });
});

function configurationFeedbackText(status: "applied" | "clamped" | "unconfirmed"): string {
  switch (status) {
    case "unconfirmed":
      return "Could not confirm mock-smart.";
    case "clamped":
      return "Requested Fusion High. The agent applied Auto.";
    case "applied":
      return "The agent applied mock-smart.";
  }
}

function storedConfigurationFeedbackMessage(
  status: "applied" | "clamped" | "unconfirmed",
): Message {
  const content = configurationFeedbackText(status);
  return {
    id: `empty-turn-${status}`,
    session_id: toSessionId("session-configuration"),
    task_id: toTaskId("task-configuration"),
    turn_id: "turn-configuration",
    author_type: "agent",
    content,
    type: "status",
    created_at: STATUS_CREATED_AT,
    metadata: {
      variant: "configuration_command_feedback",
      message: content,
      configuration_command_feedback: true,
      feedback_status: status,
      dismissible: status === "unconfirmed",
    },
  };
}

function StoredStatus({ messageId }: { messageId: string }) {
  const message = useAppStore((state) =>
    state.messages.bySession["session-configuration"]?.find((item) => item.id === messageId),
  );
  return message ? <StatusMessage comment={message} /> : null;
}

function renderStoredFeedback(message: Message) {
  const initialState: Partial<AppState> = {
    messages: {
      bySession: { [message.session_id]: [message] },
      metaBySession: {},
    },
  };
  return render(
    <StateProvider initialState={initialState}>
      <StoredStatus messageId={message.id} />
    </StateProvider>,
  );
}

describe("StatusMessage configuration command feedback", () => {
  it("lets the user dismiss an unconfirmed receipt", async () => {
    renderStoredFeedback(storedConfigurationFeedbackMessage("unconfirmed"));

    expect(screen.getByRole("status").getAttribute("aria-live")).toBe("polite");
    await screen.getByRole("button", { name: "Close" }).click();

    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });

  it("expires a confirmed receipt after five visible seconds", async () => {
    vi.useFakeTimers();
    const message = storedConfigurationFeedbackMessage("applied");
    renderStoredFeedback(message);

    expect(screen.getByRole("status")).toBeTruthy();
    await act(async () => vi.advanceTimersByTimeAsync(4_999));
    expect(screen.getByRole("status")).toBeTruthy();
    await act(async () => vi.advanceTimersByTimeAsync(1));

    expect(screen.queryByRole("status")).toBeNull();
  });

  it("expires the Copilot Auto receipt after five visible seconds", async () => {
    vi.useFakeTimers();
    renderStoredFeedback(storedConfigurationFeedbackMessage("clamped"));

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("Fusion High");
    expect(screen.getByRole("status").textContent).toContain("Auto");
    await act(async () => vi.advanceTimersByTimeAsync(5_000));

    expect(screen.queryByRole("status")).toBeNull();
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

describe("StatusMessage detached turn notices", () => {
  it.each([
    ["turn_ended_detached", "The turn ended while Kandev was unreachable."],
    [
      "paused_offline_budget",
      "The turn was paused because Kandev stayed unreachable beyond the offline budget.",
    ],
  ])("renders the %s notice from its kind", (kind, text) => {
    const comment = modelSelectionWarningMessage();
    comment.content = "ignored fallback";
    comment.metadata = { kind, episode: "stream-1:7", occurred_at: STATUS_CREATED_AT };
    render(<StatusMessage comment={comment} />);
    expect(screen.getByText(text)).toBeTruthy();
  });
});
