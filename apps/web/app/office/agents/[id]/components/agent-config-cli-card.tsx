"use client";

import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { CliProfileEditor } from "@/components/agent/cli-profile-editor";
import type { AgentProfile } from "@/lib/types/agent-profile";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

type Props = {
  agentProfileId: string;
  currentAgent: AgentProfile;
  onAgentProfileChange: (v: string) => void;
};

/**
 * Inline CLI configuration card. Shows the picker for an existing kanban
 * profile + a "Create new" / "Edit linked profile" inline editor that
 * obviates the deep-link to /settings/agents.
 */
export function AgentConfigCliCard({ agentProfileId, currentAgent, onAgentProfileChange }: Props) {
  const healthy = useHealthyAgentProfiles(agentProfileId);
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const setAgentProfiles = useAppStore((s) => s.setAgentProfiles);
  const agentProfilesState = useAppStore((s) => s.agentProfiles.items);

  const linkedProfile = useMemo(
    () => findProfile(settingsAgents, agentProfileId) ?? currentAgent,
    [settingsAgents, agentProfileId, currentAgent],
  );

  const [editorMode, setEditorMode] = useState<"closed" | "create" | "edit">("closed");
  const currentOption = useMemo(() => toCurrentAgentOption(currentAgent), [currentAgent]);
  const selected = healthy.find((p) => p.id === agentProfileId) ?? currentOption;
  const pickerOptions = useMemo(() => {
    if (healthy.some((p) => p.id === currentOption.id)) return healthy;
    return [currentOption, ...healthy];
  }, [currentOption, healthy]);

  return (
    <Card data-testid="cli-config-card">
      <CardHeader>
        <CardTitle className="text-sm">CLI Configuration</CardTitle>
        <p className="text-xs text-muted-foreground">
          Pick the CLI client + model + mode + flags this agent uses, or create a new profile
          inline.
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div>
          <Label>Agent profile</Label>
          <Select
            value={agentProfileId || "__none__"}
            onValueChange={(v) => onAgentProfileChange(v === "__none__" ? "" : v)}
          >
            <SelectTrigger className="mt-1 cursor-pointer">
              <SelectValue placeholder="Pick a CLI profile" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__none__" className="cursor-pointer">
                Unassigned
              </SelectItem>
              {pickerOptions.map((p) => (
                <SelectItem key={p.id} value={p.id} className="cursor-pointer">
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {selected && editorMode !== "edit" && (
          <ProfileSummary
            option={selected}
            onEdit={() => setEditorMode("edit")}
            onCreateNew={() => setEditorMode("create")}
          />
        )}
        {!selected && editorMode !== "create" && (
          <UnassignedHint onCreate={() => setEditorMode("create")} />
        )}

        {editorMode === "edit" && linkedProfile && (
          <InlineEditor
            mode="edit"
            profile={linkedProfile}
            onClose={() => setEditorMode("closed")}
            onSaved={(saved) => {
              optimisticUpsert(setAgentProfiles, agentProfilesState, settingsAgents, saved);
              onAgentProfileChange(saved.id);
              setEditorMode("closed");
            }}
          />
        )}
        {editorMode === "create" && (
          <InlineEditor
            mode="create"
            onClose={() => setEditorMode("closed")}
            onSaved={(saved) => {
              optimisticUpsert(setAgentProfiles, agentProfilesState, settingsAgents, saved);
              onAgentProfileChange(saved.id);
              setEditorMode("closed");
            }}
          />
        )}
      </CardContent>
    </Card>
  );
}

function toCurrentAgentOption(profile: AgentProfile): AgentProfileOption {
  const agentName = profile.agentDisplayName || profile.agentId || profile.name;
  return {
    id: profile.id,
    label: `${agentName} • ${profile.name}`,
    agent_id: profile.agentId || profile.id,
    agent_name: agentName,
    cli_passthrough: profile.cliPassthrough ?? false,
  };
}

function findProfile(
  agents: { id: string; profiles: AgentProfile[] }[],
  profileId: string,
): AgentProfile | undefined {
  for (const agent of agents) {
    const found = agent.profiles.find((p) => p.id === profileId);
    if (found) return found;
  }
  return undefined;
}

function optimisticUpsert(
  setAgentProfiles: (next: AgentProfileOption[]) => void,
  current: AgentProfileOption[],
  agents: { id: string; name: string }[],
  saved: AgentProfile,
) {
  const stub = agents.find((a) => a.id === saved.agentId) ?? {
    id: saved.agentId ?? "",
    name: saved.agentId ?? "",
  };
  const option = toAgentProfileOption(stub, saved);
  setAgentProfiles([...current.filter((p) => p.id !== option.id), option]);
}

function ProfileSummary({
  option,
  onEdit,
  onCreateNew,
}: {
  option: AgentProfileOption;
  onEdit: () => void;
  onCreateNew: () => void;
}) {
  return (
    <div className="rounded-md border bg-muted/30 p-3 space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <Badge variant="secondary">{option.agent_name}</Badge>
        {option.cli_passthrough ? <Badge variant="outline">CLI passthrough</Badge> : null}
      </div>
      <div className="flex items-center gap-3">
        <Button
          type="button"
          variant="link"
          onClick={onEdit}
          className="h-auto p-0 text-xs cursor-pointer"
        >
          Edit linked profile
        </Button>
        <Button
          type="button"
          variant="link"
          onClick={onCreateNew}
          className="h-auto p-0 text-xs cursor-pointer"
        >
          + Create a new CLI profile
        </Button>
      </div>
    </div>
  );
}

function UnassignedHint({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="space-y-2 text-xs text-muted-foreground">
      <p>
        No CLI profile selected. The agent will not be able to launch sessions until a profile is
        picked.
      </p>
      <Button
        type="button"
        variant="link"
        onClick={onCreate}
        className="h-auto p-0 cursor-pointer text-primary"
      >
        + Create a CLI profile inline
      </Button>
    </div>
  );
}

function InlineEditor({
  mode,
  profile,
  onClose,
  onSaved,
}: {
  mode: "create" | "edit";
  profile?: AgentProfile;
  onClose: () => void;
  onSaved: (saved: AgentProfile) => void;
}) {
  return (
    <div className="rounded-md border bg-background p-3">
      <CliProfileEditor
        mode={mode}
        profile={profile}
        showAdvanced
        onSaved={onSaved}
        onCancel={onClose}
      />
    </div>
  );
}
