import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@kandev/ui/card';
import Link from 'next/link';

const blogPosts = [
  {
    title: 'Introducing kandev.ai: AI-Powered Kanban for Developers',
    description: 'Learn how kandev.ai combines Kanban boards with AI agents, isolated worktrees, and Docker containers to accelerate your development workflow.',
    date: 'January 10, 2026',
    slug: 'introducing-kandev',
    author: 'Kandev Team',
  },
  {
    title: 'Running Multiple AI Agents in Parallel: Best Practices',
    description: 'Discover how to effectively manage multiple AI coding agents working simultaneously on different tasks using isolated environments and approval workflows.',
    date: 'January 5, 2026',
    slug: 'parallel-ai-agents',
    author: 'Kandev Team',
  },
];

export default function BlogPage() {
  return (
    <div className="container mx-auto max-w-6xl py-24 px-4">
      <h1 className="text-4xl font-bold mb-4">Blog</h1>
      <p className="text-lg text-muted-foreground mb-12">
        Insights and updates from the kandev.ai team.
      </p>

      <div className="grid gap-6 md:grid-cols-2">
        {blogPosts.map((post) => (
          <Link key={post.slug} href={`/blog/${post.slug}`}>
            <Card className="h-full transition-transform hover:scale-[1.02] cursor-pointer">
              <CardHeader>
                <div className="flex items-center gap-2 text-sm text-muted-foreground mb-2">
                  <span>{post.date}</span>
                  <span>•</span>
                  <span>{post.author}</span>
                </div>
                <CardTitle className="text-2xl">{post.title}</CardTitle>
              </CardHeader>
              <CardContent>
                <CardDescription className="text-base">{post.description}</CardDescription>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
