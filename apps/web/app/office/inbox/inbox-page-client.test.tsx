import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { InboxItem, OfficeMeta } from "@/lib/state/slices/office/types";

const getInboxMock = vi.hoisted(() => vi.fn());
const getMetaMock = vi.hoisted(() => vi.fn());
const listAgentProfilesMock = vi.hoisted(() => vi.fn(async () => ({ agents: [] })));
const decideApprovalMock = vi.hoisted(() => vi.fn());

const state = {
  workspaces: { activeId: "workspace-1" },
  office: {
    inboxItems: [
      {
        id: "store-item",
        type: "approval",
        title: "Store-only approval",
        status: "pending",
        createdAt: "2026-06-24T00:00:00Z",
      } satisfies InboxItem,
    ],
    meta: null,
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  decideApproval: decideApprovalMock,
  getInbox: getInboxMock,
  getMeta: getMetaMock,
  listAgentProfiles: listAgentProfilesMock,
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children, ...props }: ComponentProps<"a"> & { href: string }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

import { InboxPageClient } from "./inbox-page-client";

function inboxItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    id: "query-item",
    type: "approval",
    title: "Query approval",
    description: "From TanStack Query",
    status: "pending",
    createdAt: "2026-06-24T00:00:00Z",
    ...overrides,
  };
}

function officeMeta(): OfficeMeta {
  return {
    statuses: [],
    priorities: [],
    roles: [],
    executorTypes: [],
    skillSourceTypes: [],
    projectStatuses: [],
    agentStatuses: [],
    routineRunStatuses: [],
    inboxItemTypes: [],
    permissions: [],
    permissionDefaults: {},
  };
}

describe("InboxPageClient", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders inbox items from the TanStack Query cache", () => {
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.inbox("workspace-1"), {
      items: [inboxItem()],
      total_count: 1,
    });
    queryClient.setQueryData(qk.office.meta(), officeMeta());

    render(
      <QueryClientProvider client={queryClient}>
        <InboxPageClient initialItems={[]} initialCount={0} />
      </QueryClientProvider>,
    );

    expect(screen.getByText("Query approval")).toBeTruthy();
    expect(screen.queryByText("Store-only approval")).toBeNull();
    expect(getInboxMock).not.toHaveBeenCalled();
    expect(getMetaMock).not.toHaveBeenCalled();
  });
});
