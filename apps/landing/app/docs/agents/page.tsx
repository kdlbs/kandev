import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function AgentsDocsPage() {
  return (
    <DocsShell
      eyebrow="Features"
      title="AI Agents"
      description="Agents are containerized workers that execute tasks. They communicate with the backend using ACP, a JSON-RPC protocol over stdin/stdout."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="Agent lifecycle">
          <ol className="space-y-2 list-decimal list-inside">
            <li>Container starts with agentctl on port 9999.</li>
            <li>Backend calls <code>initialize</code> and <code>session/new</code>.</li>
            <li>Tasks are sent via <code>session/prompt</code>.</li>
            <li>Agent streams <code>session/update</code> notifications.</li>
          </ol>
        </DocsCard>
        <DocsCard title="What agents can do" variant="accent">
          <ul className="space-y-2">
            <li>Read and edit files inside the worktree</li>
            <li>Run shell commands with approval gates</li>
            <li>Stream progress updates to the UI</li>
          </ul>
        </DocsCard>
      </section>

      <DocsCard title="ACP example" variant="muted">
        <DocsCode>{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}`}</DocsCode>
      </DocsCard>
    </DocsShell>
  );
}
