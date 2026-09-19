"use client";

import { WebAppFrame } from "./web-app-frame";

export type CanvasPageProps = {
  runtimeUrl?: string | null;
  title: string;
  onLoad?: () => void;
  onError?: () => void;
};

/**
 * Provides the responsive host geometry for a plugin canvas. Runtime state is
 * rendered by WebAppFrame so status content remains in the host document.
 */
export function CanvasPage({ runtimeUrl, title, onLoad, onError }: CanvasPageProps) {
  return (
    <div
      data-testid="canvas-page"
      className="flex h-full min-h-0 min-w-0 w-full flex-1 flex-col overflow-hidden bg-background"
    >
      <WebAppFrame
        runtimeUrl={runtimeUrl}
        title={title}
        className="h-full w-full"
        onLoad={onLoad}
        onError={onError}
      />
    </div>
  );
}
