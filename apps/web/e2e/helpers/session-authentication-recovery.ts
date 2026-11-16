import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";

export const SESSION_AUTHENTICATION_PRIVACY_SENTINEL = "private-acp-auth-diagnostic-5293";

type AuthenticationCause = { operation?: string; code?: string; detail?: string };
type AuthenticationSession = {
  id: string;
  state?: string;
  agent_execution_id?: string;
  metadata?: {
    last_agent_error?: {
      stamp?: string;
      occurred_at?: string;
      causes?: AuthenticationCause[];
    };
  };
};

export type SessionAuthenticationRecoveryFixture = {
  taskId: string;
  sessionId: string;
  profileId: string;
  agentName: string;
  currentFailure(): Promise<{ stamp: string; occurredAt: string; causes: AuthenticationCause[] }>;
  completeLogin(): void;
  holdNextSessionStart(): void;
  waitForHeldSessionStart(): Promise<void>;
  releaseHeldSessionStart(): void;
  startedWorkspaces(): string[];
  loginObservations(): boolean[];
  promptRequestCount(): number;
  durableRecoveryData(): Promise<string>;
  dispose(): Promise<void>;
};

export function assertWhitespaceFreeFixtureCommandPath(scriptPath: string) {
  if (/\s/u.test(scriptPath)) {
    throw new Error("E2E backend temp paths must not contain whitespace for fixture commands");
  }
}

/** Starts a disposable ACP peer whose only login state is a file under the E2E backend temp directory. */
export async function createSessionAuthenticationRecoveryFixture(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  title: string,
): Promise<SessionAuthenticationRecoveryFixture> {
  const fixtureDir = path.join(backend.tmpDir, "session-authentication-recovery");
  const scriptPath = path.join(fixtureDir, "cursor-auth-fixture.mjs");
  const loginPath = path.join(fixtureDir, "logged-in");
  const holdPath = path.join(fixtureDir, "hold-session-start");
  const pendingPath = path.join(fixtureDir, "pending-session-start");
  const workspaceLogPath = path.join(fixtureDir, "workspaces.jsonl");
  const promptLogPath = path.join(fixtureDir, "prompts.jsonl");
  assertWhitespaceFreeFixtureCommandPath(scriptPath);
  prepareACPFixture({
    scriptPath,
    loginPath,
    holdPath,
    pendingPath,
    workspaceLogPath,
    promptLogPath,
  });

  let agentName = "";
  let profileId = "";
  let disposed = false;
  const taskWorkspacePath = path.resolve(seedData.repositoryPath);
  try {
    const agent = await apiClient.createCustomTUIAgent({
      display_name: "Cursor",
      command: `node ${scriptPath}`,
      protocol: "acp",
    });
    agentName = agent.name;
    const profile = await apiClient.createAgentProfile(agent.id, `${title} profile`, {
      cli_passthrough: false,
    });
    profileId = profile.id;
    const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      mcp_server_ids: [],
    });
    if (!task.session_id) throw new Error("authentication recovery task has no session ID");
    await pollUntil(
      async () => {
        const session = await readAuthenticationSession(apiClient, task.id, task.session_id!);
        return session?.metadata?.last_agent_error?.causes?.some(
          (cause) => cause.code === "authentication_required",
        )
          ? session
          : null;
      },
      (session) => session !== null,
      60_000,
      "Wait for the private ACP authentication cause to persist",
    );

    const fixture: SessionAuthenticationRecoveryFixture = {
      taskId: task.id,
      sessionId: task.session_id,
      profileId,
      agentName,
      async currentFailure() {
        return requireAuthenticationFailure(
          await readAuthenticationSession(apiClient, task.id, task.session_id!),
        );
      },
      completeLogin() {
        fs.writeFileSync(loginPath, "fixture account authenticated", { mode: 0o600 });
      },
      holdNextSessionStart() {
        fs.writeFileSync(holdPath, "hold", { mode: 0o600 });
      },
      async waitForHeldSessionStart() {
        await pollUntil(
          () => fs.existsSync(pendingPath),
          Boolean,
          10_000,
          "Wait for the ACP session/new request to reach its hold barrier",
        );
      },
      releaseHeldSessionStart() {
        fs.rmSync(holdPath, { force: true });
      },
      startedWorkspaces() {
        if (!fs.existsSync(workspaceLogPath)) return [];
        return fs
          .readFileSync(workspaceLogPath, "utf8")
          .split("\n")
          .filter(Boolean)
          .map((line) => (JSON.parse(line) as { cwd: string }).cwd)
          .filter((cwd) => path.resolve(cwd) === taskWorkspacePath);
      },
      loginObservations() {
        if (!fs.existsSync(workspaceLogPath)) return [];
        return fs
          .readFileSync(workspaceLogPath, "utf8")
          .split("\n")
          .filter(Boolean)
          .map((line) => JSON.parse(line) as { cwd: string; loginPresent?: boolean })
          .filter((entry) => path.resolve(entry.cwd) === taskWorkspacePath)
          .map((entry) => entry.loginPresent === true);
      },
      promptRequestCount() {
        if (!fs.existsSync(promptLogPath)) return 0;
        return fs.readFileSync(promptLogPath, "utf8").split("\n").filter(Boolean).length;
      },
      async durableRecoveryData() {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        return JSON.stringify({
          session: sessions.find((candidate) => candidate.id === task.session_id),
          messages,
        });
      },
      async dispose() {
        if (disposed) return;
        disposed = true;
        if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
        if (agentName) await apiClient.deleteCustomAgentByName(agentName).catch(() => undefined);
        fs.rmSync(fixtureDir, { recursive: true, force: true });
      },
    };
    assertSafeDurableData(await fixture.durableRecoveryData());
    return fixture;
  } catch (error) {
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (agentName) await apiClient.deleteCustomAgentByName(agentName).catch(() => undefined);
    fs.rmSync(fixtureDir, { recursive: true, force: true });
    throw error;
  }
}

function prepareACPFixture({
  scriptPath,
  loginPath,
  holdPath,
  pendingPath,
  workspaceLogPath,
  promptLogPath,
}: {
  scriptPath: string;
  loginPath: string;
  holdPath: string;
  pendingPath: string;
  workspaceLogPath: string;
  promptLogPath: string;
}) {
  fs.mkdirSync(path.dirname(scriptPath), { recursive: true });
  fs.writeFileSync(
    scriptPath,
    `import fs from "node:fs";
import readline from "node:readline";

const loginPath = ${JSON.stringify(loginPath)};
const holdPath = ${JSON.stringify(holdPath)};
const pendingPath = ${JSON.stringify(pendingPath)};
const workspaceLogPath = ${JSON.stringify(workspaceLogPath)};
const promptLogPath = ${JSON.stringify(promptLogPath)};
const send = (frame) => process.stdout.write(JSON.stringify({ jsonrpc: "2.0", ...frame }) + "\\n");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const lines = readline.createInterface({ input: process.stdin });

lines.on("line", async (line) => {
  const { id, method, params = {} } = JSON.parse(line);
  if (id === undefined) return;
  if (method === "initialize") {
    send({ id, result: {
      protocolVersion: 1,
      agentInfo: { name: "cursor-auth-recovery-fixture", version: "1.0.0" },
      agentCapabilities: {
        loadSession: true,
        mcpCapabilities: { http: true, sse: true },
        promptCapabilities: { image: false, audio: false, embeddedContext: false }
      },
      authMethods: [
        { id: "cursor_login", name: "Cursor CLI login", description: "Sign in to Cursor on the execution host." },
        {
          id: "cursor_cli_login",
          name: "Cursor command-line login",
          _meta: { "terminal-auth": { command: "cursor-agent", args: ["login"] } }
        }
      ]
    }});
    return;
  }
  if (method === "session/new" || method === "session/load") {
    const cwd = typeof params.cwd === "string" ? params.cwd : process.cwd();
    const loginPresent = fs.existsSync(loginPath);
    fs.appendFileSync(workspaceLogPath, JSON.stringify({ method, cwd, loginPresent }) + "\\n");
    if (fs.existsSync(holdPath)) {
      fs.writeFileSync(pendingPath, "held", { mode: 0o600 });
      while (fs.existsSync(holdPath)) await sleep(20);
      fs.rmSync(pendingPath, { force: true });
    }
    if (!loginPresent) {
      send({ id, error: { code: -32000, message: "Authentication required ${SESSION_AUTHENTICATION_PRIVACY_SENTINEL}" } });
      return;
    }
    send({ id, result: { sessionId: "cursor-auth-fixture-session" } });
    return;
  }
  if (method === "session/prompt") {
    fs.appendFileSync(promptLogPath, JSON.stringify({ sessionId: params.sessionId }) + "\\n");
    send({ method: "session/update", params: {
      sessionId: params.sessionId,
      update: { sessionUpdate: "agent_message_chunk", content: { type: "text", text: "Authenticated ACP fixture is ready" } }
    }});
    send({ id, result: { stopReason: "end_turn" } });
    return;
  }
  send({ id, result: {} });
});
`,
    { mode: 0o700 },
  );
}

async function readAuthenticationSession(apiClient: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId) as unknown as
    | AuthenticationSession
    | undefined;
}

function requireAuthenticationFailure(session: AuthenticationSession | undefined) {
  const failure = session?.metadata?.last_agent_error;
  if (
    !failure?.stamp ||
    !failure.occurred_at ||
    !failure.causes?.some((cause) => cause.code === "authentication_required")
  ) {
    throw new Error(
      `typed authentication failure evidence was incomplete: ${JSON.stringify(failure)}`,
    );
  }
  return { stamp: failure.stamp, occurredAt: failure.occurred_at, causes: failure.causes };
}

function assertSafeDurableData(data: string) {
  if (data.includes(SESSION_AUTHENTICATION_PRIVACY_SENTINEL)) {
    throw new Error("Raw ACP authentication diagnostics escaped into durable session data");
  }
}
