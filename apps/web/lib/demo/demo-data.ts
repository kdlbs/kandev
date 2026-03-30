// ── Pending PRs ──────────────────────────────────────────────────────────────

export type DemoPR = {
  id: string;
  number: number;
  title: string;
  repo: string;
  author: string;
  updatedAgo: string;
  additions: number;
  deletions: number;
  filesChanged: number;
};

export const DEMO_PENDING_PRS: DemoPR[] = [
  {
    id: "pr-412",
    number: 412,
    title: "feat: add circuit breaker to enrichment pipeline",
    repo: "NBCUDTC/bff",
    author: "Ana Martins",
    updatedAgo: "2h ago",
    additions: 342,
    deletions: 28,
    filesChanged: 8,
  },
  {
    id: "pr-389",
    number: 389,
    title: "fix: race condition in batch processor shutdown",
    repo: "NBCUDTC/bff",
    author: "Pedro Silva",
    updatedAgo: "4h ago",
    additions: 127,
    deletions: 45,
    filesChanged: 4,
  },
  {
    id: "pr-401",
    number: 401,
    title: "refactor: migrate config loader to v2 schema",
    repo: "NBCUDTC/bff",
    author: "João Costa",
    updatedAgo: "1d ago",
    additions: 218,
    deletions: 156,
    filesChanged: 12,
  },
];

// ── Jira Tickets ─────────────────────────────────────────────────────────────

const CURRENT_SPRINT = "Sprint 24.3";
const ASSIGNEE = "Carlos Florencio";

export type JiraStatus = "To Do" | "In Progress" | "In Review" | "Done";

export type DemoJiraTicket = {
  key: string;
  title: string;
  description: string;
  priority: "Critical" | "High" | "Medium" | "Low";
  storyPoints: number;
  sprint: string;
  assignee: string;
  status: JiraStatus;
};

export const JIRA_STATUSES: JiraStatus[] = ["To Do", "In Progress", "In Review", "Done"];

export const DEMO_JIRA_TICKETS: DemoJiraTicket[] = [
  {
    key: "BFF-1234",
    title: "Add nft scope to commit linting config",
    description: "Add nft scope to commit lint config",
    priority: "High",
    storyPoints: 8,
    sprint: CURRENT_SPRINT,
    assignee: ASSIGNEE,
    status: "To Do",
  },
  {
    key: "BFF-1235",
    title: "Add retry logic to enrichment pipeline",
    description:
      "The enrichment pipeline currently fails silently when downstream services are unavailable. Implement exponential backoff with jitter for retries, dead-letter queue for permanently failed messages, and structured logging for observability.",
    priority: "High",
    storyPoints: 5,
    sprint: CURRENT_SPRINT,
    assignee: ASSIGNEE,
    status: "To Do",
  },
  {
    key: "BFF-1236",
    title: "Migrate atom config to new schema",
    description:
      "The atom core library still uses the legacy v1 configuration schema. Migrate to v2 which supports hierarchical config, environment-aware overrides, and validation via JSON Schema. Must be backwards compatible during the migration window.",
    priority: "Medium",
    storyPoints: 5,
    sprint: CURRENT_SPRINT,
    assignee: ASSIGNEE,
    status: "In Progress",
  },
  {
    key: "BFF-1237",
    title: "Fix timeout in batch processing endpoint",
    description:
      "The /api/batch/process endpoint times out for payloads > 500 items. Root cause is likely the synchronous validation step. Move validation to async pipeline and return 202 Accepted with a job ID for polling.",
    priority: "Critical",
    storyPoints: 3,
    sprint: CURRENT_SPRINT,
    assignee: ASSIGNEE,
    status: "To Do",
  },
  {
    key: "BFF-1238",
    title: "Add OpenTelemetry tracing to core services",
    description:
      "Instrument BFF, enrichment-service, and atom with OpenTelemetry distributed tracing. Configure auto-instrumentation for HTTP/gRPC, add custom spans for business-critical paths, and export to Jaeger via OTLP.",
    priority: "Medium",
    storyPoints: 8,
    sprint: CURRENT_SPRINT,
    assignee: ASSIGNEE,
    status: "To Do",
  },
];

// ── Calendar Meetings ────────────────────────────────────────────────────────

export type DemoMeeting = {
  id: string;
  title: string;
  time: Date;
  duration: number; // minutes
  attendees: string[];
};

function nextWeekday(dayOffset: number, hour: number, minute: number): Date {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  d.setHours(hour, minute, 0, 0);
  // Skip weekends
  while (d.getDay() === 0 || d.getDay() === 6) {
    d.setDate(d.getDate() + 1);
  }
  return d;
}

export function getDemoMeetings(): DemoMeeting[] {
  return [
    {
      id: "m1",
      title: "Sprint Stand-up",
      time: nextWeekday(0, 10, 0),
      duration: 15,
      attendees: ["Team"],
    },
    {
      id: "m2",
      title: "Architecture Review",
      time: nextWeekday(0, 14, 30),
      duration: 60,
      attendees: ["Pedro", "Ana", "Tech Lead"],
    },
    {
      id: "m3",
      title: "1:1 with Tech Lead",
      time: nextWeekday(1, 11, 0),
      duration: 30,
      attendees: ["Tech Lead"],
    },
    {
      id: "m4",
      title: "Sprint Planning",
      time: nextWeekday(1, 15, 0),
      duration: 90,
      attendees: ["Team"],
    },
    {
      id: "m5",
      title: "Platform Sync",
      time: nextWeekday(2, 10, 30),
      duration: 45,
      attendees: ["Platform Team"],
    },
    {
      id: "m6",
      title: "Executive Demo",
      time: nextWeekday(2, 16, 0),
      duration: 30,
      attendees: ["CTO", "VP Eng", "Tech Lead"],
    },
  ];
}

// ── Digest Content ───────────────────────────────────────────────────────────

const ACTION_CREATE_TASK = "Create task";
const ACTION_REVIEW_PR = "Review PR";

export type DigestItem = {
  text: string;
  actionLabel?: string;
};

export type DigestSection = {
  heading: string;
  items: DigestItem[];
};

export type DigestData = {
  title: string;
  date: string;
  sections: DigestSection[];
};

export const DEMO_OUTLOOK_DIGEST: DigestData = {
  title: "Daily Digest — Outlook",
  date: "March 26, 2026",
  sections: [
    {
      heading: "Meetings Summary",
      items: [
        {
          text: "10:00 — Sprint Planning: agreed on BFF-1234 and BFF-1237 as top priorities. Pedro raised concerns about batch endpoint SLA.",
          actionLabel: ACTION_CREATE_TASK,
        },
        {
          text: "14:00 — Architecture Review: approved the move to async validation for batch processing. João will update the RFC.",
          actionLabel: ACTION_CREATE_TASK,
        },
        {
          text: "16:30 — 1:1 with Tech Lead: discussed Q2 platform reliability goals. Need to present cost analysis for agent-assisted development.",
        },
      ],
    },
    {
      heading: "Action Items",
      items: [
        {
          text: "Review PR-412 (circuit breaker) — Ana is blocked waiting for approval",
          actionLabel: ACTION_REVIEW_PR,
        },
        {
          text: "Schedule follow-up on OpenTelemetry rollout plan with SRE team",
          actionLabel: ACTION_CREATE_TASK,
        },
        {
          text: "Prepare executive demo for Friday — highlight multi-repo workflow and cost tracking",
          actionLabel: ACTION_CREATE_TASK,
        },
      ],
    },
    {
      heading: "Mentions",
      items: [
        {
          text: 'Ana Martins tagged you in "Enrichment pipeline reliability improvements" thread',
        },
        {
          text: "Platform team weekly report shared by Tech Lead — 94% SLA compliance this month",
        },
      ],
    },
  ],
};

export const DEMO_SLACK_DIGEST: DigestData = {
  title: "Daily Digest — Slack Favorites",
  date: "March 26, 2026",
  sections: [
    {
      heading: "#platform-engineering",
      items: [
        {
          text: "Pedro: batch endpoint is hitting 30s timeouts again in staging. Anyone looked at this?",
          actionLabel: ACTION_CREATE_TASK,
        },
        {
          text: "Ana: circuit breaker PR is ready for review, would appreciate eyes on the retry backoff logic",
          actionLabel: ACTION_REVIEW_PR,
        },
        {
          text: "João: atom v2 config migration is 70% done, discovered 3 undocumented config keys in production",
        },
      ],
    },
    {
      heading: "#incidents",
      items: [
        {
          text: "No active incidents. Last resolved: enrichment pipeline memory spike (2 days ago, root cause: unbounded batch size)",
        },
      ],
    },
    {
      heading: "#ai-tooling",
      items: [
        {
          text: "Tech Lead: early results from agent-assisted development look promising — 40% reduction in PR cycle time",
        },
        {
          text: "Carlos: KanDev multi-repo workflow prototype is coming along, will demo on Friday",
        },
        {
          text: "SRE Bot: agent cost report for this week: $47.20 across 28 tasks",
        },
      ],
    },
  ],
};
