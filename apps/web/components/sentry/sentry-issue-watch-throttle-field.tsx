"use client";

import type React from "react";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { type FormState, parseMaxInflightTasks } from "./sentry-issue-watch-form";

// MaxInflightTasksField caps how many open tasks a watcher may hold at once.
// Blank means uncapped; new matches are deferred to the next poll once the cap
// is reached. Mirrors the Linear watcher's throttle field.
export function MaxInflightTasksField({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const invalid = parseMaxInflightTasks(form.maxInflightTasks) === "invalid";
  return (
    <div className="space-y-1.5">
      <Label>Max in-flight tasks</Label>
      <p className="text-xs text-muted-foreground">
        Cap on open tasks created by this watcher. Leave blank for no cap. New matches are deferred
        to the next poll when the cap is reached.
      </p>
      <Input
        type="number"
        value={form.maxInflightTasks}
        onChange={(e) => setForm((p) => ({ ...p, maxInflightTasks: e.target.value }))}
        min={1}
        step={1}
        placeholder="(no cap)"
        aria-invalid={invalid}
      />
      {invalid && (
        <p className="text-xs text-destructive">Enter a positive integer or leave blank.</p>
      )}
    </div>
  );
}
