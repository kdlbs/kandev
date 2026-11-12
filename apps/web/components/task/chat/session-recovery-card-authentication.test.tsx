import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryCard } from "./session-recovery-card";
import { i18n } from "@/lib/i18n";
import ptTask from "@/src/locales/pt-pt/task.json";
import { TooltipProvider } from "@kandev/ui/tooltip";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(() => {
  cleanup();
  document.body.removeAttribute("tabindex");
});

const PROVIDER_AUTH_SUMMARY =
  "Cursor authentication is required. Sign in on the execution host before retrying.";
const CURSOR_METHOD_NAME = "Cursor CLI login";
const CURSOR_METHOD_ID = "cursor_login";
const CURSOR_PROFILE_ID = "profile-cursor";
const CURSOR_AGENT_NAME = "cursor-acp";
const AUTH_FAILURE_OCCURRED_AT = "2026-11-12T10:00:00Z";
const RECOVERY_CARD_TEST_ID = "session-recovery-card";
const COMPOSER_TEST_ID = "composer";
const RESUME_BUTTON_TEST_ID = "recovery-resume-button";
const RESTART_BUTTON_TEST_ID = "recovery-fresh-button";
const RESTORE_BUTTON_TEST_ID = "recovery-restore-workspace-button";
const NEW_BRANCH_BUTTON_TEST_ID = "recovery-new-branch-button";
const HISTORY_CONTINUATION_BUTTON_TEST_ID = "recovery-continue-from-history-button";
const NATIVE_SESSION_ID = "native-cursor-conversation";
const AUTHENTICATION_REQUIRED = "authentication_required";
const AGENT_AUTH_FAILURE = "Agent authentication is required.";

const session = {
  id: "session-auth",
  task_id: "task-auth",
  state: "FAILED",
  agent_profile_id: CURSOR_PROFILE_ID,
  metadata: {},
};
const initialState = {
  taskSessions: { items: { [session.id]: session } },
  agentProfiles: { items: [{ id: CURSOR_PROFILE_ID, agent_name: CURSOR_AGENT_NAME }] },
  availableAgents: { items: [{ name: CURSOR_AGENT_NAME, display_name: "Cursor" }] },
} as unknown as Partial<AppState>;
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  managedCloneRecoveryStamp: null,
  handleRecover: vi.fn(),
  handleManagedCloneRelocation: vi.fn().mockResolvedValue(true),
} as unknown as SessionRecoveryActions;

function renderCard(model: ActiveSessionRecovery) {
  return renderCardWithOptions(model);
}

function renderCardWithOptions(
  model: ActiveSessionRecovery,
  {
    currentSession = session,
    actionOverrides = {},
  }: {
    currentSession?: Record<string, unknown> | null;
    actionOverrides?: Partial<SessionRecoveryActions>;
  } = {},
) {
  return render(
    <StateProvider
      initialState={
        {
          ...initialState,
          taskSessions: {
            items: currentSession ? { [session.id]: currentSession } : {},
          },
        } as unknown as Partial<AppState>
      }
    >
      <TooltipProvider>
        <SessionRecoveryCard
          model={model}
          actions={{ ...actions, ...actionOverrides } as unknown as SessionRecoveryActions}
          onNewSession={vi.fn()}
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

type AuthenticationRecoveryModel = Omit<ActiveSessionRecovery, "error"> & {
  error: NonNullable<ActiveSessionRecovery["error"]>;
};

function authenticationModel(
  operation: "start" | "resume" = "start",
  phase: "bootstrap" | "resume" = operation === "start" ? "bootstrap" : "resume",
): AuthenticationRecoveryModel {
  return {
    sessionId: session.id,
    stamp: `auth-${operation}-failure`,
    kind: "generic",
    summary: AGENT_AUTH_FAILURE,
    error: {
      message: AGENT_AUTH_FAILURE,
      occurredAt: AUTH_FAILURE_OCCURRED_AT,
      phase,
      causes: [{ operation, code: AUTHENTICATION_REQUIRED }],
    },
  };
}

function FocusRecoveryHarness() {
  const [busy, setBusy] = useState(false);
  const [visible, setVisible] = useState(true);
  const handleRecover = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        setBusy(true);
        setTimeout(() => {
          document.body.setAttribute("tabindex", "-1");
          document.body.focus();
          setVisible(false);
          resolve(true);
        }, 25);
      }),
  );

  return (
    <div data-testid="session-chat">
      {visible && (
        <SessionRecoveryCard
          model={{
            sessionId: session.id,
            stamp: "auth-focus-recovery",
            kind: "generic",
            error: {
              message: AGENT_AUTH_FAILURE,
              occurredAt: AUTH_FAILURE_OCCURRED_AT,
              phase: "bootstrap",
              causes: [{ operation: "start", code: AUTHENTICATION_REQUIRED }],
            },
          }}
          actions={
            {
              ...actions,
              busyAction: busy ? "fresh_start" : null,
              handleRecover,
            } as unknown as SessionRecoveryActions
          }
          onNewSession={vi.fn()}
        />
      )}
      <div contentEditable data-testid={COMPOSER_TEST_ID} />
    </div>
  );
}

function renderFocusRecoveryHarness() {
  return render(
    <StateProvider initialState={initialState}>
      <FocusRecoveryHarness />
    </StateProvider>,
  );
}

describe("authentication recovery eligibility", () => {
  it("returns focus to the composer when an in-progress retry removes its focused control", async () => {
    renderFocusRecoveryHarness();

    const restart = screen.getByTestId(RESTART_BUTTON_TEST_ID);
    restart.focus();
    fireEvent.click(restart);

    await waitFor(() => expect(screen.queryByTestId(RECOVERY_CARD_TEST_ID)).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId(COMPOSER_TEST_ID)));
  });

  it("uses durable authentication evidence without transcript metadata or a live execution", () => {
    renderCard({
      ...authenticationModel(),
      error: { ...authenticationModel().error, executionId: "released-execution" },
    });

    const card = screen.getByTestId(RECOVERY_CARD_TEST_ID);
    expect(card.textContent).toContain(PROVIDER_AUTH_SUMMARY);
    expect(card.textContent).toContain(
      "Sign in to the provider on the execution host, then retry.",
    );
    expect(screen.queryByRole("button", { name: "Open terminal" })).toBeNull();
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
    expect(screen.getByTestId(RESTORE_BUTTON_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(RESTART_BUTTON_TEST_ID).textContent).toContain("Restart session");
    expect(card.textContent).not.toContain("released-execution");
  });

  it.each([
    ["downstream ACP session ID", { downstream_acp_session_id: NATIVE_SESSION_ID }],
    ["ACP metadata session ID", { metadata: { acp: { session_id: NATIVE_SESSION_ID } } }],
  ])(
    "keeps Resume when the current session retains %s without transcript metadata",
    (_name, identity) => {
      renderCardWithOptions(authenticationModel("resume"), {
        currentSession: { ...session, ...identity },
      });

      expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
      expect(screen.queryByTestId(RESTART_BUTTON_TEST_ID)).toBeNull();
    },
  );

  it("does not offer an identity-resetting action while native identity eligibility is unknown", () => {
    renderCardWithOptions(authenticationModel(), { currentSession: null });

    expect(screen.queryByTestId(RESTART_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
  });
});

describe("authentication recovery choices", () => {
  it("keeps branch, history, and read-only workspace recovery choices for authentication", () => {
    const handleNewBranch = vi.fn();
    const handleContinueFromHistory = vi.fn();
    const handleRestore = vi.fn();
    renderCardWithOptions(
      {
        ...authenticationModel("resume"),
        metadata: { has_resume_token: true },
      },
      {
        currentSession: {
          ...session,
          downstream_acp_session_id: NATIVE_SESSION_ID,
        },
        actionOverrides: {
          recoveryError: new Error("Workspace recovery is available."),
          branchDetails: {
            kind: "branch_unrecoverable",
            recovery_action: "resume_new_branch",
            session_id: session.id,
          } as never,
          continuationDetails: {
            kind: "session_restore_required",
            recovery_action: "continue_from_history",
            session_id: session.id,
          } as never,
          handleNewBranch,
          handleContinueFromHistory,
          handleRestore,
        },
      },
    );

    fireEvent.click(screen.getByTestId(NEW_BRANCH_BUTTON_TEST_ID));
    fireEvent.click(screen.getByTestId(HISTORY_CONTINUATION_BUTTON_TEST_ID));
    fireEvent.click(screen.getByTestId(RESTORE_BUTTON_TEST_ID));
    expect(handleNewBranch).toHaveBeenCalledOnce();
    expect(handleContinueFromHistory).toHaveBeenCalledOnce();
    expect(handleRestore).toHaveBeenCalledOnce();
  });

  it("keeps inspection busy as the prerequisite for authentication recovery", () => {
    renderCardWithOptions(authenticationModel("resume"), {
      currentSession: {
        ...session,
        downstream_acp_session_id: NATIVE_SESSION_ID,
      },
      actionOverrides: { recoveryNoticeKind: "inspection_busy" },
    });

    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RESTORE_BUTTON_TEST_ID)).toBeNull();
  });

  it("does not invent Resume while startup authentication has no native identity and inspection is busy", () => {
    renderCardWithOptions(authenticationModel(), {
      actionOverrides: {
        recoveryNotice: "Workspace recovery inspection is still running",
        recoveryNoticeKind: "inspection_busy",
      },
    });

    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RESTART_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RESTORE_BUTTON_TEST_ID)).toBeNull();
  });
});

describe("authentication method guidance", () => {
  it("shows persisted protocol method identity but does not turn it into a command", () => {
    renderCard({
      sessionId: session.id,
      stamp: "auth-with-method",
      kind: "generic",
      metadata: {
        is_auth_error: true,
        auth_methods: [{ id: CURSOR_METHOD_ID, name: CURSOR_METHOD_NAME }],
      },
      error: {
        message: AGENT_AUTH_FAILURE,
        occurredAt: AUTH_FAILURE_OCCURRED_AT,
        phase: "bootstrap",
        causes: [{ operation: "start", code: AUTHENTICATION_REQUIRED }],
      },
    });

    const card = screen.getByTestId(RECOVERY_CARD_TEST_ID);
    expect(card.textContent).toContain(CURSOR_METHOD_NAME);
    expect(card.textContent).toContain(CURSOR_METHOD_ID);
    expect(screen.queryByRole("button", { name: "Open terminal" })).toBeNull();
    expect(card.textContent).not.toContain("cursor_login login");
  });

  it("keeps advertised terminal commands copyable without launching the Kandev host shell", () => {
    const model = authenticationModel();
    renderCard({
      ...model,
      error: { ...model.error, executionId: "released-execution" },
      metadata: {
        is_auth_error: true,
        auth_methods: [
          {
            id: "terminal_login",
            name: "Cursor command-line login",
            terminal_auth: { command: "cursor", args: ["auth", "login"] },
          },
        ],
      },
    });

    expect(screen.getByText("cursor auth login")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy command" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Run in terminal" })).toBeNull();
    expect(
      screen.getByText("Sign in to the provider on the execution host, then retry."),
    ).toBeTruthy();
  });

  it("updates provider guidance when the selected locale changes", async () => {
    i18n.addResourceBundle("pt-pt", "task", ptTask, true, true);
    renderCard({
      sessionId: session.id,
      stamp: "auth-locale",
      kind: "generic",
      error: {
        message: AGENT_AUTH_FAILURE,
        occurredAt: AUTH_FAILURE_OCCURRED_AT,
        phase: "bootstrap",
        causes: [{ operation: "start", code: AUTHENTICATION_REQUIRED }],
      },
    });

    try {
      await act(() => i18n.changeLanguage("pt-pt"));
      expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).toContain(
        "É necessária autenticação em Cursor. Inicie sessão no anfitrião de execução antes de tentar novamente.",
      );
    } finally {
      cleanup();
      await i18n.changeLanguage("en");
    }
  });
});
