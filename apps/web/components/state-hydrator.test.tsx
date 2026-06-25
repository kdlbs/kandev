/* eslint-disable max-lines-per-function, sonarjs/no-duplicate-string */
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import { QueryProvider } from "@/lib/query/provider";
import type { Message } from "@/lib/types/http";
import { StateProvider } from "./state-provider";
import { StateHydrator } from "./state-hydrator";

describe("StateHydrator", () => {
  it("seeds the active TanStack Query client from route-transition state", () => {
    const queryClient = makeQueryClient();
    const message = {
      id: "message-1",
      session_id: "session-1",
      content: "hydrated",
    } as Message;

    render(
      <QueryProvider client={queryClient}>
        <StateProvider>
          <StateHydrator
            sessionId="session-1"
            initialState={{
              messages: {
                bySession: { "session-1": [message] },
                metaBySession: {
                  "session-1": { hasMore: false, oldestCursor: "message-1", isLoading: false },
                },
              },
            }}
          />
        </StateProvider>
      </QueryProvider>,
    );

    expect(queryClient.getQueryData(qk.session.messages("session-1"))).toEqual({
      messages: [message],
      hasMore: false,
      oldestCursor: "message-1",
    });
  });

  it("seeds office server-state query keys from hydrated state", () => {
    const queryClient = makeQueryClient();
    const agent = { id: "agent-1", name: "CEO" } as AppState["office"]["agentProfiles"][number];
    const project = {
      id: "project-1",
      name: "Launch",
    } as AppState["office"]["projects"][number];
    const inboxItem = {
      id: "inbox-1",
      type: "approval",
      title: "Approve",
    } as AppState["office"]["inboxItems"][number];

    render(
      <QueryProvider client={queryClient}>
        <StateProvider>
          <StateHydrator
            initialState={{
              workspaces: {
                items: [
                  {
                    id: "workspace-1",
                    name: "Office",
                  } as AppState["workspaces"]["items"][number],
                ],
                activeId: "workspace-1",
              },
              office: {
                agentProfiles: [agent],
                projects: [project],
                inboxItems: [inboxItem],
                inboxCount: 1,
                meta: { statuses: [] } as unknown as AppState["office"]["meta"],
                skills: [],
                approvals: [],
                activity: [],
                costSummary: null,
                budgetPolicies: [],
                routines: [],
                runs: [],
                dashboard: null,
                tasks: {
                  items: [],
                  filters: {
                    statuses: [],
                    priorities: [],
                    assigneeIds: [],
                    projectIds: [],
                    search: "",
                  },
                  viewMode: "list",
                  sortField: "updated",
                  sortDir: "desc",
                  groupBy: "none",
                  nestingEnabled: true,
                  isLoading: false,
                },
                isLoading: false,
                refetchTrigger: null,
                routing: { byWorkspace: {}, knownProviders: [], preview: { byWorkspace: {} } },
                providerHealth: { byWorkspace: {} },
                runAttempts: { byRunId: {} },
                agentRouting: { byAgentId: {} },
              },
            }}
          />
        </StateProvider>
      </QueryProvider>,
    );

    expect(queryClient.getQueryData(qk.office.agents("workspace-1"))).toEqual({
      agents: [agent],
    });
    expect(queryClient.getQueryData(qk.office.projects("workspace-1"))).toEqual({
      projects: [project],
    });
    expect(queryClient.getQueryData(qk.office.inbox("workspace-1"))).toEqual({
      items: [inboxItem],
      total_count: 1,
    });
  });
});
