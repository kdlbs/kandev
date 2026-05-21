"use client";

import { Component, type ErrorInfo, type ReactNode } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";

type DiffErrorBoundaryProps = {
  filePath: string;
  children: ReactNode;
};

type DiffErrorBoundaryState = {
  error: Error | null;
};

/**
 * Catches throws from @pierre/diffs renderers so a single malformed diff
 * (e.g. stale cached hunk header vs live working-tree content for an
 * untracked file that was edited after the snapshot) cannot tear down
 * the whole review list. Renders a small per-file fallback instead.
 */
export class DiffErrorBoundary extends Component<DiffErrorBoundaryProps, DiffErrorBoundaryState> {
  state: DiffErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): DiffErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("[DiffErrorBoundary]", this.props.filePath, error, info.componentStack);
  }

  componentDidUpdate(prevProps: DiffErrorBoundaryProps): void {
    if (prevProps.filePath !== this.props.filePath && this.state.error) {
      this.setState({ error: null });
    }
  }

  render(): ReactNode {
    if (this.state.error) {
      return (
        <div className="flex items-center gap-2 py-4 px-3 text-xs text-muted-foreground border-t">
          <IconAlertTriangle className="h-4 w-4 text-amber-500 shrink-0" />
          <span>
            Unable to render diff for this file. The cached diff may be out of sync with the working
            tree — refresh to retry.
          </span>
        </div>
      );
    }
    return this.props.children;
  }
}
