"use client";

import type { ReactNode } from "react";
import { Label } from "@kandev/ui/label";

/**
 * Label, control, and hint layout shared by every field in the SSH connection
 * form. Extracted so the identity-file field can render the same shape without
 * importing the form that renders it.
 */
export function SSHFieldShell({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
