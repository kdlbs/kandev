import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { OfficeMeta } from "@/lib/state/slices/office/types";
import {
  useOfficeActivityData,
  useOfficeAgentsData,
  useOfficeInboxData,
  useOfficeMetaData,
  useOfficeProjectsData,
  useOfficeRoutinesData,
  useOfficeSkillsData,
} from "./use-office-data";

type OfficeQueryClient = ReturnType<typeof makeQueryClient>;

const officeApiMocks = vi.hoisted(() => ({
  getInbox: vi.fn(),
  getMeta: vi.fn(),
  listActivity: vi.fn(),
  listAgentProfiles: vi.fn(),
  listProjects: vi.fn(),
  listRoutines: vi.fn(),
}));
const listSkillsMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    getInbox: officeApiMocks.getInbox,
    getMeta: officeApiMocks.getMeta,
    listActivity: officeApiMocks.listActivity,
    listAgentProfiles: officeApiMocks.listAgentProfiles,
    listProjects: officeApiMocks.listProjects,
    listRoutines: officeApiMocks.listRoutines,
  };
});

vi.mock("@/lib/api/domains/office-skills-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-skills-api")>(
    "@/lib/api/domains/office-skills-api",
  );
  return {
    ...actual,
    listSkills: listSkillsMock,
  };
});

function meta(overrides: Partial<OfficeMeta> = {}): OfficeMeta {
  return {
    statuses: [{ id: "todo", label: "Todo", color: "text-blue-600", order: 1 }],
    priorities: [{ id: "medium", label: "Medium", color: "text-yellow-600", order: 2, value: 2 }],
    roles: [{ id: "worker", label: "Worker", description: "Worker agent", color: "bg-blue-100" }],
    executorTypes: [{ id: "local_pc", label: "Local", description: "Local executor" }],
    skillSourceTypes: [
      {
        id: "inline",
        label: "Inline",
        readOnly: false,
      },
    ],
    projectStatuses: [{ id: "active", label: "Active", color: "bg-green-100" }],
    agentStatuses: [{ id: "idle", label: "Idle", color: "bg-neutral-400" }],
    routineRunStatuses: [{ id: "done", label: "Done", color: "bg-green-100" }],
    inboxItemTypes: [{ id: "approval", label: "Approval", icon: "shield-check" }],
    permissions: [],
    permissionDefaults: {},
    ...overrides,
  };
}

function officeWrapper(queryClient: OfficeQueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function seedOfficeSnapshots(queryClient: OfficeQueryClient, workspaceId: string) {
  const cachedAgents = { agents: [{ id: "agent-old" }] };
  const cachedProjects = { projects: [{ id: "project-old" }] };
  const cachedRoutines = { routines: [{ id: "routine-old" }] };
  const cachedSkills = { skills: [{ id: "skill-old" }] };
  const cachedInbox = { items: [{ id: "inbox-old" }], total_count: 1 };
  const cachedActivity = { activity: [{ id: "activity-old" }] };
  queryClient.setQueryData(qk.office.agents(workspaceId), cachedAgents);
  queryClient.setQueryData(qk.office.projects(workspaceId), cachedProjects);
  queryClient.setQueryData(qk.office.routines(workspaceId), cachedRoutines);
  queryClient.setQueryData(qk.office.skills(workspaceId), cachedSkills);
  queryClient.setQueryData(qk.office.inbox(workspaceId), cachedInbox);
  queryClient.setQueryData(qk.office.activity(workspaceId, "all"), cachedActivity);
  return {
    cachedActivity,
    cachedAgents,
    cachedInbox,
    cachedProjects,
    cachedRoutines,
    cachedSkills,
  };
}

function mockEmptyOfficeResponses() {
  officeApiMocks.listAgentProfiles.mockResolvedValue({ agents: [] });
  officeApiMocks.listProjects.mockResolvedValue({ projects: [] });
  officeApiMocks.listRoutines.mockResolvedValue({ routines: [] });
  officeApiMocks.getInbox.mockResolvedValue({ items: [], total_count: 0 });
  officeApiMocks.listActivity.mockResolvedValue({ activity: [] });
  listSkillsMock.mockResolvedValue({ skills: [] });
}

function renderOfficeSnapshotHooks(
  queryClient: OfficeQueryClient,
  workspaceId: string,
  initialData: "empty" | "absent",
) {
  renderHook(
    () => {
      if (initialData === "empty") {
        useOfficeAgentsData(workspaceId, []);
        useOfficeProjectsData(workspaceId, []);
        useOfficeRoutinesData(workspaceId, []);
        useOfficeSkillsData(workspaceId, []);
        useOfficeInboxData(workspaceId, [], 0);
        useOfficeActivityData(workspaceId, "all", []);
        return;
      }
      useOfficeAgentsData(workspaceId);
      useOfficeProjectsData(workspaceId);
      useOfficeRoutinesData(workspaceId);
      useOfficeSkillsData(workspaceId);
      useOfficeInboxData(workspaceId);
      useOfficeActivityData(workspaceId, "all");
    },
    { wrapper: officeWrapper(queryClient) },
  );
}

describe("useOfficeMetaData", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("reads already seeded office meta from the query cache", () => {
    const queryClient = makeQueryClient();
    const seeded = meta();
    queryClient.setQueryData(qk.office.meta(), seeded);

    const { result } = renderHook(() => useOfficeMetaData(), {
      wrapper: officeWrapper(queryClient),
    });

    expect(result.current.data).toEqual(seeded);
    expect(officeApiMocks.getMeta).not.toHaveBeenCalled();
  });

  it("seeds initial office meta into the query cache", async () => {
    const queryClient = makeQueryClient();
    const initial = meta({
      executorTypes: [{ id: "sprites", label: "Sprites", description: "Remote executor" }],
    });

    const { result } = renderHook(() => useOfficeMetaData(initial), {
      wrapper: officeWrapper(queryClient),
    });

    expect(result.current.data).toEqual(initial);
    await waitFor(() => {
      expect(queryClient.getQueryData(qk.office.meta())).toEqual(initial);
    });
    expect(officeApiMocks.getMeta).not.toHaveBeenCalled();
  });

  it("seeds explicit empty office snapshots over cached lists", async () => {
    const workspaceId = "workspace-1";
    const queryClient = makeQueryClient();
    seedOfficeSnapshots(queryClient, workspaceId);
    mockEmptyOfficeResponses();

    renderOfficeSnapshotHooks(queryClient, workspaceId, "empty");

    await waitFor(() => {
      expect(queryClient.getQueryData(qk.office.agents(workspaceId))).toEqual({ agents: [] });
      expect(queryClient.getQueryData(qk.office.projects(workspaceId))).toEqual({ projects: [] });
      expect(queryClient.getQueryData(qk.office.routines(workspaceId))).toEqual({ routines: [] });
      expect(queryClient.getQueryData(qk.office.skills(workspaceId))).toEqual({ skills: [] });
      expect(queryClient.getQueryData(qk.office.inbox(workspaceId))).toEqual({
        items: [],
        total_count: 0,
      });
      expect(queryClient.getQueryData(qk.office.activity(workspaceId, "all"))).toEqual({
        activity: [],
      });
    });
  });

  it("does not overwrite cached office snapshots when initial route data is absent", async () => {
    const workspaceId = "workspace-1";
    const queryClient = makeQueryClient();
    const cached = seedOfficeSnapshots(queryClient, workspaceId);

    renderOfficeSnapshotHooks(queryClient, workspaceId, "absent");

    await waitFor(() => {
      expect(queryClient.getQueryData(qk.office.agents(workspaceId))).toBe(cached.cachedAgents);
      expect(queryClient.getQueryData(qk.office.projects(workspaceId))).toBe(cached.cachedProjects);
      expect(queryClient.getQueryData(qk.office.routines(workspaceId))).toBe(cached.cachedRoutines);
      expect(queryClient.getQueryData(qk.office.skills(workspaceId))).toBe(cached.cachedSkills);
      expect(queryClient.getQueryData(qk.office.inbox(workspaceId))).toBe(cached.cachedInbox);
      expect(queryClient.getQueryData(qk.office.activity(workspaceId, "all"))).toBe(
        cached.cachedActivity,
      );
    });
  });
});
