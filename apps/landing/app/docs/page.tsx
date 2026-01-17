import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function DocsPage() {
  return (
    <DocsShell
      eyebrow="Getting Started"
      title="Documentation"
      description="Learn how kandev.ai orchestrates AI agents across isolated worktrees, Docker containers, and a WebSocket-first control plane."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="What is kandev.ai?" variant="accent">
          <p>
            kandev.ai is a WebSocket-first platform that pairs Kanban task management with AI-assisted execution.
            Every task runs inside a dedicated git worktree and a Docker container, with approvals for sensitive
            operations.
          </p>
        </DocsCard>
        <DocsCard title="Why teams use it">
          <ul className="space-y-2">
            <li>Ship in parallel without branch conflicts.</li>
            <li>Keep execution auditable and reviewable.</li>
            <li>Stream progress updates in real time.</li>
          </ul>
        </DocsCard>
      </section>

      <DocsCard title="Core workflow" variant="muted">
        <p>From task creation to approval, kandev.ai follows a consistent loop.</p>
        <DocsCode>{`1. Create task on the board
2. Start agent container via agentctl
3. Initialize ACP session
4. Stream progress over WebSocket
5. Review and approve output`}</DocsCode>
      </DocsCard>

      <section className="grid gap-4 md:grid-cols-3">
        <DocsCard title="Workspaces">
          <p>Group repositories, boards, and team settings.</p>
        </DocsCard>
        <DocsCard title="Boards" variant="accent">
          <p>Model your workflow with Kanban columns.</p>
        </DocsCard>
        <DocsCard title="Tasks">
          <p>Define the scope, constraints, and agent requirements.</p>
        </DocsCard>
      </section>
    </DocsShell>
  );
}
