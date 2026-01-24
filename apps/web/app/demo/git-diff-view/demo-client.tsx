'use client';

import { useState } from 'react';
import { ThemeProvider } from 'next-themes';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { Button } from '@kandev/ui/button';
import { GitDiffViewer } from '@/components/git-diff-view';

// Sample diffs for demo
const BUTTON_DIFF_OLD = `import React from 'react';

interface ButtonProps {
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
}

export function Button({ children, onClick, disabled }: ButtonProps) {
  return (
    <button onClick={onClick} disabled={disabled}>
      {children}
    </button>
  );
}`;

const BUTTON_DIFF_NEW = `import React from 'react';
import { cn } from '../lib/utils';

interface ButtonProps {
  children: React.ReactNode;
  onClick?: (event: React.MouseEvent) => void;
  variant?: 'primary' | 'secondary';
  disabled?: boolean;
}

export function Button({
  children,
  onClick,
  variant = 'primary',
  disabled
}: ButtonProps) {
  return (
    <button className={cn('btn', \`btn-\${variant}\`)} onClick={onClick} disabled={disabled}>
      {children}
    </button>
  );
}`;

const FORMAT_DIFF_OLD = '';
const FORMAT_DIFF_NEW = `export function formatDate(date: Date): string {
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
}

export function formatCurrency(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(amount);
}`;

type DiffExample = 'button' | 'newfile';

const DIFFS: Record<
  DiffExample,
  { label: string; oldContent: string; newContent: string; path: string; description: string }
> = {
  button: {
    label: 'Button Component',
    oldContent: BUTTON_DIFF_OLD,
    newContent: BUTTON_DIFF_NEW,
    path: 'src/Button.tsx',
    description: 'Standard diff with additions and deletions',
  },
  newfile: {
    label: 'New File',
    oldContent: FORMAT_DIFF_OLD,
    newContent: FORMAT_DIFF_NEW,
    path: 'src/utils/format.ts',
    description: 'Shows a newly created file (all additions)',
  },
};

function GitDiffViewDemoContent() {
  const [selectedDiff, setSelectedDiff] = useState<DiffExample>('button');
  const currentDiff = DIFFS[selectedDiff];

  return (
    <div className="min-h-screen bg-background p-6">
      <div className="max-w-6xl mx-auto space-y-6">
        {/* Header */}
        <div className="space-y-2">
          <h1 className="text-2xl font-bold">GitDiffViewer Demo</h1>
          <p className="text-muted-foreground">
            Diff viewer with inline comments and multi-line selection.
          </p>
        </div>

        {/* Diff selector */}
        <div className="flex flex-wrap gap-2">
          {Object.entries(DIFFS).map(([key, { label }]) => (
            <Button
              key={key}
              variant={selectedDiff === key ? 'default' : 'outline'}
              size="sm"
              onClick={() => setSelectedDiff(key as DiffExample)}
            >
              {label}
            </Button>
          ))}
        </div>

        {/* Description */}
        <div className="text-sm text-muted-foreground bg-muted/50 px-3 py-2 rounded">
          {currentDiff.description}
        </div>

        {/* Instructions */}
        <div className="text-xs text-muted-foreground bg-primary/5 border border-primary/20 px-3 py-2 rounded space-y-1">
          <p>
            <strong>Single line:</strong> Click the{' '}
            <span className="font-mono bg-muted px-1 rounded">+</span> button on any line to add a
            comment.
          </p>
          <p>
            <strong>Multi-line:</strong> Click and drag from one line to another to select a range,
            then add your comment.
          </p>
        </div>

        {/* Diff viewer */}
        <GitDiffViewer
          oldContent={currentDiff.oldContent}
          newContent={currentDiff.newContent}
          filePath={currentDiff.path}
          language="tsx"
          defaultViewMode="split"
          enableComments
        />

        {/* Features list */}
        <div className="p-4 bg-muted rounded-lg space-y-3 text-sm">
          <h3 className="font-medium">Features:</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <h4 className="font-medium text-muted-foreground mb-1">Built-in Features</h4>
              <ul className="list-disc list-inside space-y-0.5 text-muted-foreground">
                <li>Syntax highlighting</li>
                <li>Split and unified view modes</li>
                <li>Light and dark theme support</li>
                <li>Add widget (+) buttons on hover</li>
              </ul>
            </div>
            <div>
              <h4 className="font-medium text-muted-foreground mb-1">Comments</h4>
              <ul className="list-disc list-inside space-y-0.5 text-muted-foreground">
                <li>Click + to open inline comment widget</li>
                <li>Click and drag to select multiple lines</li>
                <li>Visual overlay shows selected range</li>
                <li>Comments persisted to localStorage</li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export function GitDiffViewDemo() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <TooltipProvider>
        <GitDiffViewDemoContent />
      </TooltipProvider>
    </ThemeProvider>
  );
}
