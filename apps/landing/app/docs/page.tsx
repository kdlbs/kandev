export default function DocsPage() {
  return (
    <div className="prose prose-slate dark:prose-invert max-w-none">
      <div className="not-prose mb-8">
        <h1 className="text-4xl font-bold mb-4">Getting Started with kandev.ai</h1>
        <p className="text-xl text-muted-foreground">
          kandev.ai combines Kanban task management with AI-powered development workflows. Manage tasks visually,
          execute them with AI agents in isolated environments, and ship faster with confidence.
        </p>
      </div>

      <div className="not-prose mb-8 p-6 bg-muted/50 rounded-lg border border-border">
        <h2 className="text-2xl font-bold mb-3">What is kandev.ai?</h2>
        <p className="text-muted-foreground">
          kandev.ai is a development tool that bridges project management and AI-assisted coding. Each task on your
          Kanban board can be executed by AI agents working in isolated git worktrees and Docker containers.
        </p>
      </div>

      <h2 className="text-2xl font-bold mt-8 mb-4">Key Features</h2>
      <div className="grid gap-4 not-prose mb-8">
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Visual Task Management</h3>
          <p className="text-sm text-muted-foreground">Organize development work with Kanban boards</p>
        </div>
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Isolated Worktrees</h3>
          <p className="text-sm text-muted-foreground">Each task gets its own git worktree for parallel development</p>
        </div>
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Local Docker Execution</h3>
          <p className="text-sm text-muted-foreground">AI agents run in secure Docker containers on your machine</p>
        </div>
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Multiple AI Agents</h3>
          <p className="text-sm text-muted-foreground">Support for Claude Code, OpenAI Codex, Augment, and more</p>
        </div>
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Approval Workflows</h3>
          <p className="text-sm text-muted-foreground">Review and approve agent changes before merging</p>
        </div>
        <div className="p-4 border border-border rounded-lg">
          <h3 className="font-semibold mb-2">Multiple Workspaces</h3>
          <p className="text-sm text-muted-foreground">Organize projects by client, team, or product</p>
        </div>
      </div>

      <h2 className="text-2xl font-bold mt-8 mb-4">How It Works</h2>
      <div className="not-prose space-y-4 mb-8">
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">1</div>
          <div>
            <h3 className="font-semibold mb-1">Create a Task</h3>
            <p className="text-sm text-muted-foreground">Add a task to your Kanban board with a description of what needs to be done</p>
          </div>
        </div>
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">2</div>
          <div>
            <h3 className="font-semibold mb-1">Configure Repository</h3>
            <p className="text-sm text-muted-foreground">Select a repository and base branch for the task</p>
          </div>
        </div>
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">3</div>
          <div>
            <h3 className="font-semibold mb-1">Choose AI Agent</h3>
            <p className="text-sm text-muted-foreground">Select which AI agent should work on the task</p>
          </div>
        </div>
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">4</div>
          <div>
            <h3 className="font-semibold mb-1">Isolated Environment</h3>
            <p className="text-sm text-muted-foreground">kandev creates an isolated git worktree and Docker container</p>
          </div>
        </div>
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">5</div>
          <div>
            <h3 className="font-semibold mb-1">AI Execution</h3>
            <p className="text-sm text-muted-foreground">The AI agent executes the task in the secure environment</p>
          </div>
        </div>
        <div className="flex gap-4">
          <div className="flex-shrink-0 w-8 h-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold">6</div>
          <div>
            <h3 className="font-semibold mb-1">Review & Approve</h3>
            <p className="text-sm text-muted-foreground">Review the changes and approve them for merging</p>
          </div>
        </div>
      </div>

      <div className="not-prose mb-8 p-6 bg-primary/10 rounded-lg border border-primary/20">
        <h2 className="text-2xl font-bold mb-3">Why kandev.ai?</h2>
        <p className="text-muted-foreground mb-4">
          Traditional development workflows require constant context switching. kandev.ai eliminates this by:
        </p>
        <ul className="space-y-2">
          <li className="flex items-start gap-2">
            <span className="text-primary">✓</span>
            <span className="text-sm">Visualizing all work in progress on a single board</span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-primary">✓</span>
            <span className="text-sm">Enabling parallel development without branch conflicts</span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-primary">✓</span>
            <span className="text-sm">Automating routine tasks with AI agents</span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-primary">✓</span>
            <span className="text-sm">Maintaining security through isolated execution environments</span>
          </li>
        </ul>
      </div>

      <div className="not-prose mb-8 p-6 bg-accent/10 rounded-lg border border-accent/20">
        <h2 className="text-2xl font-bold mb-3">Security & Control</h2>
        <p className="text-muted-foreground">
          Every operation performed by AI agents is logged and requires approval. Agents run in isolated Docker
          containers with restricted permissions, ensuring your codebase and system remain secure.
        </p>
      </div>

      <div className="not-prose mt-12 p-8 bg-gradient-to-br from-primary/10 to-accent/10 rounded-lg border border-border text-center">
        <h2 className="text-2xl font-bold mb-4">Ready to Get Started?</h2>
        <p className="text-muted-foreground mb-6">
          Check out our guides to install and configure kandev.ai
        </p>
        <div className="flex gap-4 justify-center">
          <a
            href="/docs/installation"
            className="inline-flex items-center justify-center px-6 py-3 rounded-md bg-primary text-primary-foreground font-medium hover:bg-primary/90 transition-colors"
          >
            Installation Guide
          </a>
          <a
            href="/docs/quick-start"
            className="inline-flex items-center justify-center px-6 py-3 rounded-md border border-border bg-background hover:bg-muted font-medium transition-colors"
          >
            Quick Start Tutorial
          </a>
        </div>
      </div>
    </div>
  );
}
