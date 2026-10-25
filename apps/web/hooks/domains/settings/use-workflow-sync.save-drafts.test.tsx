import { act, cleanup, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  useWorkflowSync,
  type WorkflowSyncController,
  type WorkflowSyncFormState,
} from "./use-workflow-sync";
import { config, Providers, transport } from "./workflow-sync.lifetime.test-helpers";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson,
}));

let wire: ReturnType<typeof transport>;
const TOAST = "toast-container";
beforeEach(() => {
  wire = transport();
  fetchJson.mockImplementation(wire.fetch);
});
afterEach(async () => {
  cleanup();
  await act(async () => wire.drain());
  vi.restoreAllMocks();
});

async function mount() {
  const hook = renderHook(() => useWorkflowSync("A"), { wrapper: Providers });
  await waitFor(() => expect(hook.result.current.loading).toBe(false));
  return hook;
}

function draft(sync: WorkflowSyncController) {
  return { form: { ...sync.form }, url: sync.url, urlInvalid: sync.urlInvalid };
}

function submit(sync: WorkflowSyncController) {
  const held = wire.hold("POST");
  let outcome!: Promise<boolean>;
  act(() => {
    outcome = sync.handleSave();
  });
  expect(wire.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  return { held, outcome };
}

const fields: [keyof WorkflowSyncFormState, WorkflowSyncFormState[keyof WorkflowSyncFormState]][] =
  [
    ["repo_owner", "other-team"],
    ["repo_name", "other-repo"],
    ["project_path", "group/inactive-project"],
    ["branch", "features/newer"],
    ["path", "newer/workflows"],
    ["interval_seconds", 601],
    ["poll_enabled", false],
  ];

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.1, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.2
describe("workflow source raw acknowledgement", () => {
  it.each(fields)(
    "records accepted canonical config while preserving complete raw edits: %s",
    async (key, value) => {
      const hook = await mount();
      const { held, outcome } = submit(hook.result.current);
      act(() => hook.result.current.update(key, value));
      const current = draft(hook.result.current);
      const accepted = config("A", { branch: "server-accepted" });
      await act(async () => {
        held.resolve(accepted);
        expect(await outcome).toBe(true);
      });
      expect(draft(hook.result.current)).toEqual(current);
      expect(hook.result.current.config).toEqual(accepted);
      expect(hook.result.current.saving).toBe(false);
      expect(screen.getByTestId(TOAST).textContent).toMatch(/saved/i);
    },
  );

  it.each([
    "https://github.com/new-team/new-repo/tree/next/new-directory",
    "an invalid repository link",
    "git@github.com:team/A.git",
    "",
  ])("preserves raw link %s even when parsing leaves the old identifiers", async (url) => {
    const hook = await mount();
    const { held, outcome } = submit(hook.result.current);
    act(() => hook.result.current.setUrlInput(url));
    const current = draft(hook.result.current);
    await act(async () => {
      held.resolve(config("A"));
      expect(await outcome).toBe(true);
    });
    expect(draft(hook.result.current)).toEqual(current);
  });

  it("preserves a provider switch and all batched raw input changes", async () => {
    const hook = await mount();
    const { held, outcome } = submit(hook.result.current);
    await act(async () => {
      hook.result.current.setProvider("gitlab");
      hook.result.current.setUrlInput(
        "https://git.internal/team/sub/project/-/tree/next/workflows",
      );
      hook.result.current.update("branch", "features/retained");
      hook.result.current.update("path", "retained/path");
      hook.result.current.update("interval_seconds", 17);
      hook.result.current.update("poll_enabled", false);
      held.resolve(config("A"));
      expect(await outcome).toBe(true);
    });
    expect(hook.result.current.form).toEqual({
      provider: "gitlab",
      repo_owner: "",
      repo_name: "",
      project_path: "team/sub/project",
      branch: "features/retained",
      path: "retained/path",
      interval_seconds: 17,
      poll_enabled: false,
    });
    expect(hook.result.current.url).toBe(
      "https://git.internal/team/sub/project/-/tree/next/workflows",
    );
    expect(hook.result.current.config?.provider).toBe("github");
  });
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.3, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.4
describe("submitted draft equality", () => {
  it("keeps accepted success truthful when presentation notification throws", async () => {
    const hook = await mount();
    const held = wire.hold("POST");
    let outcome!: Promise<boolean>;
    act(() => {
      outcome = hook.result.current.handleSave(() => {
        throw new Error("presentation-only");
      });
    });
    const accepted = config("A", { branch: "accepted" });
    await act(async () => {
      held.resolve(accepted);
      expect(await outcome).toBe(true);
    });
    expect(hook.result.current.config).toEqual(accepted);
    expect(hook.result.current.form.branch).toBe("accepted");
    expect(screen.getByTestId(TOAST).textContent).toMatch(/saved/i);
    expect(screen.getByTestId(TOAST).textContent).not.toContain("presentation-only");
  });

  it("adopts canonical editable values for an unchanged raw draft", async () => {
    const hook = await mount();
    act(() => {
      hook.result.current.setUrlInput("git@github.com:team/A.git");
      hook.result.current.update("branch", " release ");
      hook.result.current.update("path", " workflows/ ");
    });
    const { held, outcome } = submit(hook.result.current);
    const body = JSON.parse(wire.requests.find((r) => r.method === "POST")!.body!);
    expect(body).toMatchObject({ branch: "release", path: "workflows/" });
    await act(async () => {
      held.resolve(config("A", { branch: "release", path: "workflows" }));
      expect(await outcome).toBe(true);
    });
    expect(hook.result.current.form.branch).toBe("release");
    expect(hook.result.current.form.path).toBe("workflows");
    expect(hook.result.current.url).toBe("https://github.com/team/A");
  });

  it("preserves a revert to old baseline and submits that draft next", async () => {
    const hook = await mount();
    act(() => hook.result.current.update("branch", "submitted"));
    const { held, outcome } = submit(hook.result.current);
    act(() => hook.result.current.update("branch", "main"));
    await act(async () => {
      held.resolve(config("A", { branch: "submitted" }));
      expect(await outcome).toBe(true);
    });
    expect(hook.result.current.form.branch).toBe("main");
    expect(hook.result.current.config?.branch).toBe("submitted");
    const second = wire.hold("POST");
    let saved!: Promise<boolean>;
    act(() => {
      saved = hook.result.current.handleSave();
    });
    expect(JSON.parse(wire.requests.filter((r) => r.method === "POST")[1].body!).branch).toBe(
      "main",
    );
    await act(async () => {
      second.resolve(config("A"));
      expect(await saved).toBe(true);
    });
  });

  it("allows adoption after editing away and back to submitted raw values", async () => {
    const hook = await mount();
    act(() => hook.result.current.update("branch", " submitted "));
    const { held, outcome } = submit(hook.result.current);
    act(() => hook.result.current.update("branch", "away"));
    act(() => hook.result.current.update("branch", " submitted "));
    await act(async () => {
      held.resolve(config("A", { branch: "submitted" }));
      expect(await outcome).toBe(true);
    });
    expect(hook.result.current.form.branch).toBe("submitted");
  });
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.5, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.6
describe("failure and independent current owners", () => {
  it("keeps complete current raw edits after rejection and retries their payload", async () => {
    const hook = await mount();
    const { held, outcome } = submit(hook.result.current);
    act(() => {
      hook.result.current.setProvider("gitlab");
      hook.result.current.setUrlInput("group/next-project");
      hook.result.current.update("branch", "retry-branch");
      hook.result.current.update("path", "retry-path");
    });
    const current = draft(hook.result.current);
    await act(async () => {
      held.reject(new Error("current-save-rejected"));
      expect(await outcome).toBe(false);
    });
    expect(draft(hook.result.current)).toEqual(current);
    expect(hook.result.current.saving).toBe(false);
    expect(screen.getByTestId(TOAST).textContent).toContain("current-save-rejected");
    const retried = wire.hold("POST");
    let saved!: Promise<boolean>;
    act(() => {
      saved = hook.result.current.handleSave();
    });
    const body = JSON.parse(wire.requests.filter((r) => r.method === "POST")[1].body!);
    expect(body).toEqual({
      provider: "gitlab",
      project_path: "group/next-project",
      branch: "retry-branch",
      path: "retry-path",
      interval_seconds: 300,
      poll_enabled: true,
    });
    await act(async () => {
      retried.resolve(config("A", { ...body, repo_owner: "", repo_name: "" }));
      expect(await saved).toBe(true);
    });
  });

  it("keeps independent hook/store drafts local", async () => {
    const first = await mount();
    const second = await mount();
    const { held, outcome } = submit(first.result.current);
    act(() => second.result.current.setUrlInput("invalid second draft"));
    const other = draft(second.result.current);
    await act(async () => {
      held.resolve(config("A", { branch: "accepted-first" }));
      expect(await outcome).toBe(true);
    });
    expect(first.result.current.form.branch).toBe("accepted-first");
    expect(draft(second.result.current)).toEqual(other);
    expect(second.result.current.config?.branch).toBe("main");
  });
});
