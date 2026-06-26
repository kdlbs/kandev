import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { OfficeMeta } from "@/lib/state/slices/office/types";
import { useOfficeMetaData } from "./use-office-data";

const getMetaMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    getMeta: getMetaMock,
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
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    expect(result.current.data).toEqual(seeded);
    expect(getMetaMock).not.toHaveBeenCalled();
  });

  it("seeds initial office meta into the query cache", async () => {
    const queryClient = makeQueryClient();
    const initial = meta({
      executorTypes: [{ id: "sprites", label: "Sprites", description: "Remote executor" }],
    });

    const { result } = renderHook(() => useOfficeMetaData(initial), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    expect(result.current.data).toEqual(initial);
    await waitFor(() => {
      expect(queryClient.getQueryData(qk.office.meta())).toEqual(initial);
    });
    expect(getMetaMock).not.toHaveBeenCalled();
  });
});
