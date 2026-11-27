import type { JiraProject, JiraStatus, JiraTicket } from "@/lib/types/jira";
import type { DemoHttpResponse } from "./protocol";

// i18n-exempt: browser demo fixture data is intentionally literal demo content
const PROJECTS: JiraProject[] = [{ id: "demo-jira-platform", key: "PLAT", name: "Acme Platform" }];
// i18n-exempt: browser demo fixture data is intentionally literal demo content
const STATUSES: JiraStatus[] = [
  { id: "open", name: "Open", statusCategory: "new" },
  { id: "progress", name: "In Progress", statusCategory: "indeterminate" },
  { id: "review", name: "In Review", statusCategory: "indeterminate" },
  { id: "done", name: "Done", statusCategory: "done" },
];

// i18n-exempt: browser demo fixture data is intentionally literal demo content
function seedTickets(): JiraTicket[] {
  return [
    ["PLAT-142", "Record privileged account changes", "review"],
    ["PLAT-137", "Fix intermittent checkout timeouts", "progress"],
    ["PLAT-150", "Improve first-time workspace empty states", "open"],
    ["PLAT-131", "Preserve device sessions during token rotation", "done"],
  ].map(([key, summary, status]) => {
    const selected = STATUSES.find((candidate) => candidate.id === status)!;
    return {
      key,
      summary,
      description: `${summary}. Include regression coverage and document the review criteria.`,
      statusId: selected.id,
      statusName: selected.name,
      statusCategory: selected.statusCategory,
      projectKey: "PLAT",
      issueType: "Task",
      priority: "High",
      assigneeName: "Demo User",
      reporterName: "Mira",
      updated: "2026-07-18T12:00:00.000Z",
      url: `https://acme.atlassian.net/browse/${key}`,
      transitions: STATUSES.map((next) => ({
        id: next.id,
        name: next.name,
        toStatusId: next.id,
        toStatusName: next.name,
      })),
    };
  });
}

export function createDemoJiraRuntime() {
  const tickets = seedTickets();
  return {
    route(
      path: string,
      method: string,
      params: URLSearchParams,
      input: Record<string, unknown>,
    ): DemoHttpResponse | null {
      if (method === "GET" && path === "/api/v1/jira/projects") return json({ projects: PROJECTS });
      if (method === "GET" && /^\/api\/v1\/jira\/projects\/[^/]+\/statuses$/.test(path))
        return json({ statuses: STATUSES });
      if (method === "GET" && path === "/api/v1/jira/tickets")
        return searchTickets(tickets, params);
      const match = path.match(/^\/api\/v1\/jira\/tickets\/([^/]+)(\/transitions)?$/);
      if (!match) return null;
      const ticket = tickets.find((candidate) => candidate.key === decodeURIComponent(match[1]));
      if (!ticket) return json({ error: "Ticket not found" }, 404);
      if (method === "GET" && !match[2]) return json(ticket);
      if (method !== "POST" || !match[2]) return null;
      const status = STATUSES.find((candidate) => candidate.id === input.transitionId);
      if (!status) return json({ error: "Transition not found" }, 404);
      ticket.statusId = status.id;
      ticket.statusName = status.name;
      ticket.statusCategory = status.statusCategory;
      return json({ transitioned: true });
    },
  };
}

function searchTickets(tickets: JiraTicket[], params: URLSearchParams) {
  const filtered = tickets.filter((ticket) => matchesDemoQuery(ticket, params.get("jql") ?? ""));
  const maxResults = Math.max(1, Math.min(100, Number(params.get("max_results")) || 25));
  const offset = Math.max(0, Number(params.get("page_token")) || 0);
  const end = offset + maxResults;
  return json({
    tickets: filtered.slice(offset, end),
    maxResults,
    isLast: end >= filtered.length,
    ...(end < filtered.length ? { nextPageToken: String(end) } : {}),
  });
}

// Only the clauses produced by the demo's filter controls are simulated, not arbitrary JQL.
function matchesDemoQuery(ticket: JiraTicket, query: string) {
  for (const [field, value] of [
    ["project", ticket.projectKey],
    ["status", ticket.statusName],
    ["key", ticket.key],
  ]) {
    const clause = query.match(
      new RegExp(
        `\\b${field}\\s+(?:in\\s*\\(([^)]+)\\)|=\\s*("(?:\\\\.|[^"\\\\])*"|[\\w-]+))`,
        "i",
      ),
    );
    if (!clause) continue;
    const values = (clause[1] ?? clause[2]).match(/"(?:\\.|[^"\\])*"|[\w-]+/g) ?? [];
    if (!values.some((item) => decodeQuoted(item).toLowerCase() === value.toLowerCase()))
      return false;
  }
  const text = query.match(/\btext\s*~\s*("(?:\\.|[^"\\])*")/i)?.[1];
  if (
    text &&
    !`${ticket.key} ${ticket.summary} ${ticket.description}`
      .toLowerCase()
      .includes(decodeQuoted(text).toLowerCase())
  )
    return false;
  return !/\bassignee\s+is\s+EMPTY/i.test(query) || !ticket.assigneeName;
}

function decodeQuoted(value: string): string {
  return value.startsWith('"') ? JSON.parse(value) : value;
}

function json(body: unknown, status = 200): DemoHttpResponse {
  return { status, headers: { "Content-Type": "application/json" }, body };
}
