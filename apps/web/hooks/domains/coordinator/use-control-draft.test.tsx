import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { CoordinatorSettings } from "@/lib/api/domains/coordinator-api";

const getMock = vi.fn();
const putMock = vi.fn();
const toastError = vi.fn();
let contributor: {
  isDirty: boolean;
  canSave?: boolean;
  invalidReason?: string;
  save: () => Promise<void>;
  discard: () => void;
} | null = null;

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  getCoordinatorSettings: (...args: unknown[]) => getMock(...args),
  putCoordinatorSettings: (...args: unknown[]) => putMock(...args),
}));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (c: typeof contributor) => {
    contributor = c;
  },
}));
const listSetsMock = vi.fn();
const listReposMock = vi.fn();
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositorySets: (...a: unknown[]) => listSetsMock(...a),
  listRepositories: (...a: unknown[]) => listReposMock(...a),
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { error: (...a: unknown[]) => toastError(...a) } }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { useControlDraft } from "./use-control-draft";

function settings(over: Partial<CoordinatorSettings> = {}): CoordinatorSettings {
  return {
    policy: {
      actions: {
        create_task: "requires_approval",
        start_agent: "denied",
        message: "denied",
        move: "denied",
        resume: "denied",
        stop: "denied",
      },
    },
    policy_revision: 1,
    watches: { scope: "selected", workflow_ids: ["a", "b"] },
    ...over,
  };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function mount() {
  return renderHook(() =>
    useControlDraft({ workspaceId: "w1", coordinatorId: "c1", canManage: true }),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  contributor = null;
});

describe("useControlDraft", () => {
  it("sends one PUT with only the changed member", async () => {
    getMock.mockResolvedValue(settings());
    putMock.mockImplementation(async (_w, _c, req) =>
      settings({ policy: req.policy ?? settings().policy }),
    );
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setAction("message", "requires_approval"));
    expect(contributor?.isDirty).toBe(true);
    await act(async () => contributor?.save());
    expect(putMock).toHaveBeenCalledTimes(1);
    const req = putMock.mock.calls[0][2];
    expect(req.watches).toBeUndefined();
    expect(req.policy.actions.message).toBe("requires_approval");
    expect(result.current.policyDirty).toBe(false);
  });

  it("does not let a read sent before the PUT undo the save", async () => {
    const first = deferred<CoordinatorSettings>();
    getMock.mockResolvedValueOnce(settings());
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));

    getMock.mockReturnValueOnce(first.promise);
    act(() => result.current.retry());
    act(() => result.current.setAction("move", "requires_approval"));
    putMock.mockResolvedValue(
      settings({
        policy: { actions: { ...settings().policy.actions, move: "requires_approval" } },
      }),
    );
    await act(async () => contributor?.save());
    await act(async () => first.resolve(settings()));
    expect(result.current.stored?.actions.move).toBe("requires_approval");
    expect(result.current.draft?.actions.move).toBe("requires_approval");
  });

  it("keeps an edited action across a re-read while following the others", async () => {
    getMock.mockResolvedValueOnce(settings());
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setAction("message", "requires_approval"));
    getMock.mockResolvedValueOnce(
      settings({
        policy: { actions: { ...settings().policy.actions, resume: "requires_approval" } },
      }),
    );
    await act(async () => result.current.retry());
    await waitFor(() => expect(result.current.stored?.actions.resume).toBe("requires_approval"));
    expect(result.current.draft?.actions.message).toBe("requires_approval");
    expect(result.current.draft?.actions.resume).toBe("requires_approval");
  });

  it("keeps an edit made while the PUT is in flight", async () => {
    getMock.mockResolvedValueOnce(settings());
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setAction("message", "requires_approval"));
    const inFlight = deferred<CoordinatorSettings>();
    putMock.mockReturnValueOnce(inFlight.promise);
    let saving!: Promise<void>;
    act(() => {
      saving = contributor!.save();
    });
    act(() => result.current.setAction("move", "requires_approval"));
    await act(async () => {
      inFlight.resolve(
        settings({
          policy: { actions: { ...settings().policy.actions, message: "requires_approval" } },
        }),
      );
      await saving;
    });
    expect(result.current.draft?.actions.move).toBe("requires_approval");
    expect(result.current.policyDirty).toBe(true);
  });
});

describe("useControlDraft failures and validity", () => {
  it("names a 400 inline and rethrows; other failures toast", async () => {
    getMock.mockResolvedValue(settings());
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setAction("message", "requires_approval"));
    putMock.mockRejectedValueOnce(
      new ApiError("bad", 400, {
        error: "bad",
        field: "policy.actions.message",
        code: "invalid_setting",
      }),
    );
    await act(async () => {
      await expect(contributor?.save()).rejects.toBeTruthy();
    });
    expect(result.current.fieldError?.code).toBe("invalid_setting");
    expect(toastError).not.toHaveBeenCalled();

    putMock.mockRejectedValueOnce(new ApiError("down", 500, null));
    await act(async () => {
      await expect(contributor?.save()).rejects.toBeTruthy();
    });
    expect(toastError).toHaveBeenCalledTimes(1);
  });

  it("names a 409 not_eligible raise refusal inline instead of toasting", async () => {
    getMock.mockResolvedValue(settings());
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setAction("create_task", "automatic"));
    putMock.mockRejectedValueOnce(
      new ApiError("not_eligible", 409, {
        error: "not_eligible",
        code: "not_eligible",
        field: "policy.actions.create_task",
        condition: "volume",
      }),
    );
    await act(async () => {
      await expect(contributor?.save()).rejects.toBeTruthy();
    });
    expect(result.current.fieldError?.code).toBe("not_eligible");
    expect(result.current.fieldError?.field).toBe("policy.actions.create_task");
    expect(toastError).not.toHaveBeenCalled();
  });

  it("blocks Save for an edited zero-board draft but not for an unedited stored empty set", async () => {
    getMock.mockResolvedValueOnce(settings({ watches: { scope: "selected", workflow_ids: [] } }));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(contributor?.canSave).toBe(true);
    act(() => result.current.setAction("message", "requires_approval"));
    expect(contributor?.canSave).toBe(true);
    act(() => result.current.setWatches({ scope: "selected", workflowIds: ["a"] }));
    act(() => result.current.setWatches({ scope: "selected", workflowIds: [] }));
    expect(contributor?.canSave).toBe(true);
  });

  it("blocks Save with the reason once a stored board set is edited to zero boards", async () => {
    getMock.mockResolvedValueOnce(
      settings({ watches: { scope: "selected", workflow_ids: ["a"] } }),
    );
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.setWatches({ scope: "selected", workflowIds: [] }));
    expect(contributor?.canSave).toBe(false);
    expect(contributor?.invalidReason).toBe("coordinator:watchesKeepOneBoard");
  });

  it("reports a first-load failure as error and recovers on retry", async () => {
    getMock.mockRejectedValueOnce(new Error("x"));
    const { result } = mount();
    await waitFor(() => expect(result.current.status).toBe("error"));
    getMock.mockResolvedValueOnce(settings());
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("ready"));
  });
});

const selectedProjects = (ids: string[]) =>
  settings({
    projects_config: {
      scope: "selected",
      entries: ids.map((id) => ({ kind: "repository" as const, id })),
      include_no_repository: false,
    },
  });
const repos = (ids: string[]) => ({
  repositories: ids.map((id) => ({ id, name: id })),
});

describe("useControlDraft projects scope", () => {
  it("blocks Save for a selected draft while the project listing failed", async () => {
    listSetsMock.mockRejectedValue(new Error("down"));
    listReposMock.mockRejectedValue(new Error("down"));
    getMock.mockResolvedValue(selectedProjects(["r1", "r2"]));
    const { result } = mount();
    await waitFor(() => expect(result.current.projectsRead.status).toBe("error"));
    act(() =>
      result.current.setProjects({
        ...result.current.draft!.projects!,
        entries: [{ kind: "repository", id: "r1" }],
      }),
    );
    expect(contributor?.isDirty).toBe(true);
    expect(contributor?.canSave).toBe(false);
  });

  it("refreshes the listings and prunes the draft on projects_foreign_entry", async () => {
    listSetsMock.mockResolvedValue({ repository_sets: [] });
    listReposMock.mockResolvedValueOnce(repos(["r1", "r2"]));
    getMock.mockResolvedValue(selectedProjects(["r1", "r2"]));
    const { result } = mount();
    await waitFor(() => expect(result.current.projectsRead.status).toBe("ready"));
    act(() =>
      result.current.setProjects({
        ...result.current.draft!.projects!,
        entries: [
          { kind: "repository", id: "r1" },
          { kind: "repository", id: "r2" },
        ],
      }),
    );
    act(() =>
      result.current.setProjects({
        ...result.current.draft!.projects!,
        includeNoRepository: true,
      }),
    );
    putMock.mockRejectedValueOnce(
      new ApiError("gone", 400, {
        field: "projects",
        code: "projects_foreign_entry",
        error: "gone",
      }),
    );
    listReposMock.mockResolvedValueOnce(repos(["r1"]));
    await act(async () => {
      await contributor?.save().catch(() => {});
    });
    await waitFor(() =>
      expect(result.current.draft?.projects?.entries).toEqual([{ kind: "repository", id: "r1" }]),
    );
    expect(listReposMock).toHaveBeenCalledTimes(2);
    expect(result.current.fieldError?.code).toBe("projects_foreign_entry");
  });

  it("lets an unrelated save through after the project list is emptied", async () => {
    listSetsMock.mockResolvedValue({ repository_sets: [] });
    listReposMock.mockResolvedValue(repos(["r1"]));
    getMock.mockResolvedValue(selectedProjects([]));
    putMock.mockImplementation(async (_w, _c, req) =>
      settings({
        policy: req.policy ?? settings().policy,
        projects_config: { scope: "selected", entries: [], include_no_repository: false },
      }),
    );
    const { result } = mount();
    await waitFor(() => expect(result.current.projectsRead.status).toBe("ready"));
    act(() => result.current.setAction("message", "requires_approval"));
    expect(contributor?.canSave).toBe(true);
    await act(async () => contributor?.save());
    expect(putMock.mock.calls[0][2].projects).toBeUndefined();
  });
});

describe("useControlDraft projects listing", () => {
  it("keeps a stored repository entry that belongs to a listed set when pruning", async () => {
    listSetsMock.mockResolvedValue({
      repository_sets: [{ id: "s1", name: "S", repositories: [{ repository_id: "r1" }] }],
    });
    listReposMock.mockResolvedValue(repos(["r1"]));
    getMock.mockResolvedValue(selectedProjects(["r1", "gone"]));
    putMock.mockRejectedValueOnce(
      new ApiError("gone", 400, {
        field: "projects",
        code: "projects_foreign_entry",
        error: "gone",
      }),
    );
    const { result } = mount();
    await waitFor(() => expect(result.current.projectsRead.status).toBe("ready"));
    act(() =>
      result.current.setProjects({ ...result.current.draft!.projects!, includeNoRepository: true }),
    );
    await act(async () => {
      await contributor?.save().catch(() => {});
    });
    await waitFor(() =>
      expect(result.current.draft?.projects?.entries).toEqual([{ kind: "repository", id: "r1" }]),
    );
  });

  it("lets a policy-only save through while the project listing failed", async () => {
    listSetsMock.mockRejectedValue(new Error("down"));
    listReposMock.mockRejectedValue(new Error("down"));
    getMock.mockResolvedValue(selectedProjects(["r1"]));
    const { result } = mount();
    await waitFor(() => expect(result.current.projectsRead.status).toBe("error"));
    act(() => result.current.setAction("message", "requires_approval"));
    expect(contributor?.isDirty).toBe(true);
    expect(contributor?.canSave).toBe(true);
  });
});
