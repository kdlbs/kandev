"use client";

import type { ReactNode } from "react";

/** A section heading over one card that reports whether anything inside changed. */
export function EditorSection({
  title,
  description,
  dirty,
  testId,
  children,
}: {
  title: string;
  description?: string;
  dirty: boolean;
  testId: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-2" data-testid={testId}>
      <div>
        <h3 className="text-base font-medium">{title}</h3>
        {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      </div>
      <div
        className="space-y-4 rounded-lg border bg-card p-4"
        data-settings-dirty={dirty}
        data-settings-dirty-level="container"
      >
        {children}
      </div>
    </section>
  );
}
