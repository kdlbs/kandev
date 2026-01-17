'use client';

import { useState } from 'react';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import { IconCheck, IconCopy } from '@tabler/icons-react';

export function HeroSection() {
  const [copied, setCopied] = useState(false);

  const copyCommand = () => {
    navigator.clipboard.writeText('npx kandev');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <section className="relative overflow-hidden">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_top,_rgba(109,40,217,0.12),_transparent_55%)]" />
      <div className="container mx-auto max-w-6xl flex flex-col items-center gap-8 px-4 pt-16 pb-16 md:pb-24">
        <Badge variant="secondary" className="cursor-pointer border border-border px-6 py-3">
          <span className="text-sm">kandev.ai – AI-powered Kanban for developers</span>
        </Badge>
        <h1 className="max-w-4xl text-center text-4xl font-bold tracking-tight sm:text-5xl md:text-6xl lg:text-7xl">
          Kanban meets <span className="text-primary">AI development</span>
        </h1>
        <p className="max-w-2xl text-center text-lg text-muted-foreground md:text-xl">
          Manage development tasks with isolated git worktrees, secure Docker execution, and AI agents.
          Ship faster without breaking your workflow.
        </p>
        <div className="flex flex-col items-center gap-4 sm:flex-row">
          <Button size="lg" asChild className="cursor-pointer">
            <a href="#features">Get Started</a>
          </Button>
          <Button size="lg" variant="outline" asChild className="cursor-pointer">
            <a href="#demo">Watch Demo</a>
          </Button>
        </div>
        <div className="mt-8 w-full max-w-2xl">
          <div className="flex items-center justify-center gap-2 rounded-lg border border-border bg-card p-4">
            <code className="flex-1 text-sm font-mono text-foreground">npx kandev</code>
            <Button
              size="icon"
              variant="ghost"
              onClick={copyCommand}
              className="cursor-pointer"
            >
              {copied ? (
                <IconCheck className="h-4 w-4 text-green-500" />
              ) : (
                <IconCopy className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>
        <div className="mt-8 w-full max-w-5xl">
          <div className="aspect-video rounded-lg border border-border bg-muted flex items-center justify-center">
            <p className="text-muted-foreground">Video placeholder</p>
          </div>
        </div>
      </div>
    </section>
  );
}
