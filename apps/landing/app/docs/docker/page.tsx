import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function DockerDocsPage() {
  return (
    <DocsShell
      eyebrow="Features"
      title="Docker Execution"
      description="Every agent runs inside a Docker container to keep execution isolated from your host machine. Containers are created per task and destroyed after completion."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="What is mounted">
          <ul className="space-y-2">
            <li><code>/workspace</code> — your repository worktree</li>
            <li><code>/root/.augment/sessions</code> — agent session storage (when enabled)</li>
          </ul>
        </DocsCard>
        <DocsCard title="Why containers" variant="accent">
          <ol className="space-y-2 list-decimal list-inside">
            <li>Consistent, reproducible environments</li>
            <li>Security boundaries for file and network access</li>
            <li>Easy cleanup after task completion</li>
          </ol>
        </DocsCard>
      </section>

      <DocsCard title="Sample run (conceptual)" variant="muted">
        <DocsCode>{`docker run -it --rm \
  -v /path/to/repo:/workspace \
  -e TASK_DESCRIPTION="Fix the login bug" \
  kandev/augment-agent:latest`}</DocsCode>
      </DocsCard>
    </DocsShell>
  );
}
