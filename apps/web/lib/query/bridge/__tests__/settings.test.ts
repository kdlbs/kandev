import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerSettingsBridge } from "../settings";
import { qk } from "@/lib/query/keys";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { Executor } from "@/lib/types/http";
import type { InstallJob } from "@/lib/types/settings";

// Shared constants to satisfy sonar no-duplicate-string rule
const TS_JAN_2024 = "2024-01-01T00:00:00Z";
const TS_FEB_2024 = "2024-01-02T00:00:00Z";
const AGENT_CLAUDE_CODE = "claude-code";
const EVT_USER_SETTINGS_UPDATED = "user.settings.updated";

// ---------------------------------------------------------------------------
// Fake WS client — captures handlers registered via ws.on()
// ---------------------------------------------------------------------------
type Handler = (message: { payload: unknown }) => void;

interface FakeWs {
  on: (type: string, handler: Handler) => () => void;
  emit: (type: string, payload: unknown) => void;
  listeners: Map<string, Set<Handler>>;
}

function createFakeWs(): FakeWs {
  const listeners = new Map<string, Set<Handler>>();

  const on = vi.fn((type: string, handler: Handler) => {
    const set = listeners.get(type) ?? new Set();
    set.add(handler);
    listeners.set(type, set);
    return () => {
      listeners.get(type)?.delete(handler);
    };
  });

  function emit(type: string, payload: unknown) {
    for (const handler of listeners.get(type) ?? []) {
      handler({ payload } as { payload: unknown });
    }
  }

  return { on, emit, listeners };
}

function createTestClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
}

describe("registerSettingsBridge — executors", () => {
  let ws: FakeWs;
  let qc: QueryClient;
  let cleanup: () => void;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    cleanup = registerSettingsBridge(ws as never, qc);
    // Seed initial executors
    qc.setQueryData(qk.settings.executors(), [
      {
        id: "exec-1",
        name: "Exec 1",
        type: "local_pc",
        status: "active",
        is_system: false,
        created_at: TS_JAN_2024,
        updated_at: TS_JAN_2024,
      } as Executor,
    ]);
  });

  it("adds a new executor on executor.created", () => {
    ws.emit("executor.created", {
      id: "exec-2",
      name: "Exec 2",
      type: "local_docker",
      status: "active",
      is_system: false,
      created_at: TS_FEB_2024,
      updated_at: TS_FEB_2024,
    });
    const executors = qc.getQueryData<Executor[]>(qk.settings.executors());
    expect(executors?.map((e) => e.id)).toContain("exec-2");
  });

  it("updates an executor on executor.updated", () => {
    ws.emit("executor.updated", { id: "exec-1", name: "Exec 1 Updated", status: "active" });
    const executors = qc.getQueryData<Executor[]>(qk.settings.executors());
    expect(executors?.find((e) => e.id === "exec-1")?.name).toBe("Exec 1 Updated");
  });

  it("removes an executor on executor.deleted", () => {
    ws.emit("executor.deleted", { id: "exec-1" });
    const executors = qc.getQueryData<Executor[]>(qk.settings.executors());
    expect(executors?.map((e) => e.id)).not.toContain("exec-1");
  });

  it("cleanup unregisters all handlers", () => {
    cleanup();
    // After cleanup, emitting should not throw and data should remain unchanged
    const before = qc.getQueryData<Executor[]>(qk.settings.executors())?.length ?? 0;
    ws.emit("executor.deleted", { id: "exec-1" });
    const after = qc.getQueryData<Executor[]>(qk.settings.executors())?.length ?? 0;
    expect(after).toBe(before);
  });
});

describe("registerSettingsBridge — secrets", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerSettingsBridge(ws as never, qc);
    qc.setQueryData(qk.settings.secrets(), [
      {
        id: "sec-1",
        name: "MY_SECRET",
        has_value: true,
        created_at: "",
        updated_at: "",
      } satisfies SecretListItem,
    ]);
  });

  it("adds a secret on secrets.created", () => {
    const newSecret: SecretListItem = {
      id: "sec-2",
      name: "NEW_SECRET",
      has_value: true,
      created_at: "",
      updated_at: "",
    };
    ws.emit("secrets.created", newSecret);
    const secrets = qc.getQueryData<SecretListItem[]>(qk.settings.secrets());
    expect(secrets?.map((s) => s.id)).toContain("sec-2");
  });

  it("updates a secret on secrets.updated", () => {
    ws.emit("secrets.updated", {
      id: "sec-1",
      name: "MY_SECRET_UPDATED",
      has_value: true,
      created_at: "",
      updated_at: "",
    });
    const secrets = qc.getQueryData<SecretListItem[]>(qk.settings.secrets());
    expect(secrets?.find((s) => s.id === "sec-1")?.name).toBe("MY_SECRET_UPDATED");
  });

  it("removes a secret on secrets.deleted", () => {
    ws.emit("secrets.deleted", { id: "sec-1" });
    const secrets = qc.getQueryData<SecretListItem[]>(qk.settings.secrets());
    expect(secrets?.map((s) => s.id)).not.toContain("sec-1");
  });
});

describe("registerSettingsBridge — install jobs", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerSettingsBridge(ws as never, qc);
    qc.setQueryData(qk.settings.installJobs(), [] as InstallJob[]);
  });

  it("upserts a job on agent.install.started", () => {
    const job: InstallJob = {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      status: "running",
      started_at: TS_JAN_2024,
    };
    ws.emit("agent.install.started", job);
    const jobs = qc.getQueryData<InstallJob[]>(qk.settings.installJobs());
    expect(jobs?.find((j) => j.job_id === "job-1")).toBeDefined();
  });

  it("appends output on agent.install.output", () => {
    const job: InstallJob = {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      status: "running",
      started_at: TS_JAN_2024,
    };
    qc.setQueryData(qk.settings.installJobs(), [job]);
    ws.emit("agent.install.output", {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      chunk: "Installing...\n",
    });
    const jobs = qc.getQueryData<InstallJob[]>(qk.settings.installJobs());
    expect(jobs?.find((j) => j.job_id === "job-1")?.output).toBe("Installing...\n");
  });

  it("does not exceed 64KB cap on output", () => {
    const job: InstallJob = {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      status: "running",
      started_at: TS_JAN_2024,
      output: "x".repeat(64 * 1024 - 5),
    };
    qc.setQueryData(qk.settings.installJobs(), [job]);
    ws.emit("agent.install.output", {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      chunk: "overflow",
    });
    const jobs = qc.getQueryData<InstallJob[]>(qk.settings.installJobs());
    const output = jobs?.find((j) => j.job_id === "job-1")?.output ?? "";
    expect(output.length).toBeLessThanOrEqual(64 * 1024);
    expect(output.endsWith("overflow")).toBe(true);
  });

  it("drops stale job events from an older job_id", () => {
    const newerJob: InstallJob = {
      job_id: "job-2",
      agent_name: AGENT_CLAUDE_CODE,
      status: "running",
      started_at: TS_FEB_2024,
    };
    const olderJob: InstallJob = {
      job_id: "job-1",
      agent_name: AGENT_CLAUDE_CODE,
      status: "succeeded",
      started_at: TS_JAN_2024,
    };
    qc.setQueryData(qk.settings.installJobs(), [newerJob]);
    // Emit the older job — should NOT replace the newer one
    ws.emit("agent.install.started", olderJob);
    const jobs = qc.getQueryData<InstallJob[]>(qk.settings.installJobs());
    expect(jobs?.find((j) => j.agent_name === AGENT_CLAUDE_CODE)?.job_id).toBe("job-2");
  });
});

describe("registerSettingsBridge — agent profiles", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerSettingsBridge(ws as never, qc);
  });

  it("removes a profile on agent.profile.deleted", () => {
    qc.setQueryData(qk.settings.agentProfiles(), [
      {
        id: "prof-1",
        label: "Claude • Default",
        agent_id: "agent-1",
        agent_name: "claude",
        cli_passthrough: false,
      },
    ]);
    ws.emit("agent.profile.deleted", {
      profile: { id: "prof-1", agent_id: "agent-1", name: "Default" },
    });
    const profiles = qc.getQueryData<unknown[]>(qk.settings.agentProfiles());
    expect(profiles?.find((p) => (p as { id: string }).id === "prof-1")).toBeUndefined();
  });
});

describe("registerSettingsBridge — user settings", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerSettingsBridge(ws as never, qc);
  });

  it("maps a user.settings.updated payload into the mapped cache", () => {
    ws.emit(EVT_USER_SETTINGS_UPDATED, {
      user_id: "u-1",
      workspace_id: "ws-1",
      repository_ids: ["r-1"],
      preferred_shell: "/bin/zsh",
      chat_submit_key: "enter",
      changes_panel_layout: "tree",
      terminal_link_behavior: "browser_panel",
    });
    const settings = qc.getQueryData<{
      preferredShell: string | null;
      chatSubmitKey: string;
      changesPanelLayout: string;
      terminalLinkBehavior: string;
      loaded: boolean;
    }>(qk.settings.userSettings());
    expect(settings?.preferredShell).toBe("/bin/zsh");
    expect(settings?.chatSubmitKey).toBe("enter");
    expect(settings?.changesPanelLayout).toBe("tree");
    expect(settings?.terminalLinkBehavior).toBe("browser_panel");
    expect(settings?.loaded).toBe(true);
  });

  it("preserves navigation fields already in the cache (workspaceId, workflowId)", () => {
    qc.setQueryData(qk.settings.userSettings(), {
      workspaceId: "ws-keep",
      workflowId: "wf-keep",
      repositoryIds: ["r-keep"],
      preferredShell: null,
      shellOptions: [],
      defaultEditorId: null,
      enablePreviewOnClick: false,
      chatSubmitKey: "cmd_enter",
      reviewAutoMarkOnScroll: true,
      showReleaseNotification: true,
      releaseNotesLastSeenVersion: null,
      lspAutoStartLanguages: [],
      lspAutoInstallLanguages: [],
      lspServerConfigs: {},
      savedLayouts: [],
      sidebarViews: [],
      defaultUtilityAgentId: null,
      keyboardShortcuts: {},
      terminalLinkBehavior: "new_tab",
      terminalFontFamily: null,
      terminalFontSize: null,
      changesPanelLayout: "flat",
      kanbanViewMode: null,
      voiceMode: {
        enabled: true,
        engine: "auto",
        language: "auto",
        mode: "toggle",
        autoSend: false,
        whisperWebModel: "base",
      },
      loaded: true,
    });
    ws.emit(EVT_USER_SETTINGS_UPDATED, {
      user_id: "u-1",
      workspace_id: "ws-broadcast",
      repository_ids: [],
      default_editor_id: "ed-1",
    });
    const settings = qc.getQueryData<{
      workspaceId: string | null;
      workflowId: string | null;
      repositoryIds: string[];
      defaultEditorId: string | null;
    }>(qk.settings.userSettings());
    expect(settings?.workspaceId).toBe("ws-keep");
    expect(settings?.workflowId).toBe("wf-keep");
    expect(settings?.repositoryIds).toEqual(["r-keep"]);
    expect(settings?.defaultEditorId).toBe("ed-1");
  });
});

describe("registerSettingsBridge — user settings voice mode", () => {
  let ws: FakeWs;
  let qc: QueryClient;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    registerSettingsBridge(ws as never, qc);
  });

  it("maps the voice_mode payload into the voiceMode cache field", () => {
    ws.emit(EVT_USER_SETTINGS_UPDATED, {
      user_id: "u-1",
      workspace_id: "ws-1",
      repository_ids: [],
      voice_mode: {
        enabled: false,
        engine: "whisperWeb",
        language: "pt-PT",
        mode: "hold",
        auto_send: true,
        whisper_web_model: "small",
      },
    });
    const settings = qc.getQueryData<{
      voiceMode: {
        enabled: boolean;
        engine: string;
        language: string;
        mode: string;
        autoSend: boolean;
        whisperWebModel: string;
      };
    }>(qk.settings.userSettings());
    expect(settings?.voiceMode).toEqual({
      enabled: false,
      engine: "whisperWeb",
      language: "pt-PT",
      mode: "hold",
      autoSend: true,
      whisperWebModel: "small",
    });
  });

  it("defaults voiceMode when the payload omits voice_mode", () => {
    ws.emit(EVT_USER_SETTINGS_UPDATED, {
      user_id: "u-1",
      workspace_id: "ws-1",
      repository_ids: [],
    });
    const settings = qc.getQueryData<{ voiceMode: { enabled: boolean; engine: string } }>(
      qk.settings.userSettings(),
    );
    // voice mode is opt-out: a row without voice_mode defaults to enabled.
    expect(settings?.voiceMode.enabled).toBe(true);
    expect(settings?.voiceMode.engine).toBe("auto");
  });
});
