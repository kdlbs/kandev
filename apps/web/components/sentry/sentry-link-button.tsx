"use client";

import { useCallback, useEffect, useState } from "react";
import { IconLink } from "@tabler/icons-react";
import { toast } from "sonner";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { getSentryIssue, listSentryInstances } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { SENTRY_SHORT_ID_RE } from "./sentry-issue-common";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type SentryLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
  // instanceId pins the lookup to one instance; omit it to auto-use the sole
  // configured instance or offer a picker when several exist.
  instanceId?: string;
};

// useSentryInstanceChoice resolves which Sentry instance the lookup targets:
// the explicit prop when pinned, the sole configured instance automatically,
// otherwise a user pick from the loaded list.
function useSentryInstanceChoice(explicit?: string) {
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  const [selected, setSelected] = useState<string>("");
  useEffect(() => {
    if (explicit) return;
    let cancelled = false;
    listSentryInstances()
      .then((list) => {
        if (cancelled) return;
        setInstances(list);
        if (list.length === 1) setSelected(list[0].id);
      })
      .catch(() => {
        if (!cancelled) setInstances([]);
      });
    return () => {
      cancelled = true;
    };
  }, [explicit]);
  return {
    instances,
    selected: explicit ?? selected,
    setSelected,
    showPicker: !explicit && instances.length > 1,
  };
}

function InstancePicker({
  instances,
  selected,
  onChange,
}: {
  instances: SentryConfig[];
  selected: string;
  onChange: (id: string) => void;
}) {
  return (
    <div className="space-y-1">
      <Label className="text-[11px] text-muted-foreground">Sentry instance</Label>
      <Select value={selected || undefined} onValueChange={onChange}>
        <SelectTrigger className="h-8 text-xs" data-testid="sentry-link-instance-select">
          <SelectValue placeholder="Select Sentry instance" />
        </SelectTrigger>
        <SelectContent>
          {instances.map((i) => (
            <SelectItem key={i.id} value={i.id}>
              {i.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// SentryLinkButton attaches a Sentry issue to an existing task by prepending
// its short ID to the title ("PROJ-123: ...").
export function SentryLinkButton({
  taskId,
  workspaceId,
  taskTitle,
  instanceId,
}: SentryLinkButtonProps) {
  const available = useSentryAvailable();
  const { instances, selected, setSelected, showPicker } = useSentryInstanceChoice(instanceId);

  const buildLinkedTitle = useCallback(
    (key: string) => {
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9_-]*-\d+:\s*/, "");
      return stripped ? `${key}: ${stripped}` : key;
    },
    [taskTitle],
  );

  // Require taskTitle to be loaded (== null catches null + undefined but allows
  // an empty string): otherwise linking would overwrite the real title with
  // just the Sentry key while the title is still in flight.
  if (!available || !taskId || !workspaceId || taskTitle == null) return null;

  return (
    <ValidatedPopover
      triggerStyle="outline-with-label"
      triggerIcon={<IconLink className="h-4 w-4" />}
      triggerLabel="Link Sentry"
      tooltip="Link this task to a Sentry issue"
      headline="Link to Sentry issue"
      placeholder="PROJ-123 or paste issue URL"
      aboveInput={
        showPicker ? (
          <InstancePicker instances={instances} selected={selected} onChange={setSelected} />
        ) : undefined
      }
      extractKey={(raw) => {
        const upper = raw.toUpperCase().trim();
        return SENTRY_SHORT_ID_RE.test(upper)
          ? upper
          : (upper.match(/[A-Z][A-Z0-9_-]*-\d+/)?.[0] ?? null);
      }}
      validationHint="Paste a Sentry issue URL or short ID (PROJ-123)"
      fetch={async (key) => {
        if (!selected) throw new Error("Select a Sentry instance");
        const issue = await getSentryIssue(selected, key);
        await updateTask(taskId, { title: buildLinkedTitle(key) });
        return issue;
      }}
      onSuccess={(key) => {
        toast.success(`Linked to ${key}`);
      }}
      submitLabel="Link"
      submittingLabel="Linking..."
    />
  );
}
