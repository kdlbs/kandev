"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { IconSearch, IconUserCheck, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { listProjectMembers } from "@/lib/api/domains/gitlab-api";
import type { GitLabMRUser, GitLabProjectMember } from "@/lib/types/gitlab";
import { isCurrentIdentityRequest } from "@/hooks/domains/gitlab/request-identity";

export function toggleMemberId(ids: number[], id: number): number[] {
  return ids.includes(id) ? ids.filter((candidate) => candidate !== id) : [...ids, id];
}

type Props = {
  workspaceId: string;
  host: string;
  project: string;
  label: "Reviewers" | "Assignees";
  current: GitLabMRUser[];
  busy: boolean;
  onSave: (ids: number[]) => Promise<boolean>;
};

function SelectedMemberBadges({
  members,
  label,
  onRemove,
}: {
  members: Array<Pick<GitLabProjectMember, "id" | "username" | "name">>;
  label: Props["label"];
  onRemove: (id: number) => void;
}) {
  if (members.length === 0)
    return <span className="text-xs text-muted-foreground">None assigned</span>;
  return members.map((member) => (
    <Badge key={member.id} variant="secondary" className="gap-1">
      {member.username}
      <button
        type="button"
        className="flex h-11 w-11 cursor-pointer items-center justify-center sm:h-6 sm:w-6"
        aria-label={`Remove ${member.username} from ${label.toLowerCase()}`}
        onClick={() => onRemove(member.id)}
      >
        <IconX className="h-3 w-3" />
      </button>
    </Badge>
  ));
}

function useProjectMemberSearch({
  workspaceId,
  host,
  project,
}: Pick<Props, "workspaceId" | "host" | "project">) {
  const [query, setQuery] = useState("");
  const [members, setMembers] = useState<GitLabProjectMember[]>([]);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestGeneration = useRef(0);
  const identity = `${workspaceId}\0${host}\0${project}`;
  const currentIdentity = useRef(identity);
  currentIdentity.current = identity;
  const isActive = (generation: number, requestIdentity: string) =>
    isCurrentIdentityRequest(
      generation,
      requestGeneration.current,
      requestIdentity,
      currentIdentity.current,
    );

  useEffect(() => {
    requestGeneration.current += 1;
    setQuery("");
    setMembers([]);
    setError(null);
    setSearching(false);
  }, [identity]);

  const search = async () => {
    const generation = ++requestGeneration.current;
    const requestIdentity = identity;
    setSearching(true);
    setError(null);
    try {
      const result = await listProjectMembers(workspaceId, project, query.trim(), host);
      if (isActive(generation, requestIdentity)) setMembers(result);
    } catch (searchError) {
      if (isActive(generation, requestIdentity)) {
        setError(searchError instanceof Error ? searchError.message : "Member search failed");
      }
    } finally {
      if (isActive(generation, requestIdentity)) setSearching(false);
    }
  };
  return { query, setQuery, members, searching, error, search };
}

export function MRReviewerControl({
  workspaceId,
  host,
  project,
  label,
  current,
  busy,
  onSave,
}: Props) {
  const initialIds = useMemo(() => current.map((member) => member.id), [current]);
  const [selectedIds, setSelectedIds] = useState(initialIds);
  const { query, setQuery, members, searching, error, search } = useProjectMemberSearch({
    workspaceId,
    host,
    project,
  });

  useEffect(() => setSelectedIds(initialIds), [initialIds]);

  const changed = selectedIds.join(",") !== initialIds.join(",");
  const knownMembers = new Map<number, Pick<GitLabProjectMember, "id" | "username" | "name">>();
  for (const member of [...current, ...members]) knownMembers.set(member.id, member);
  const selectedMembers = selectedIds
    .map((id) => knownMembers.get(id))
    .filter((member): member is Pick<GitLabProjectMember, "id" | "username" | "name"> => !!member);
  return (
    <section className="space-y-2" aria-label={label}>
      <div className="flex items-center justify-between gap-2">
        <h4 className="text-xs font-semibold">{label}</h4>
        <Button
          size="sm"
          variant="outline"
          className="h-11 cursor-pointer sm:h-8"
          disabled={busy || !changed}
          onClick={() => void onSave(selectedIds)}
        >
          <IconUserCheck className="mr-1 h-3.5 w-3.5" /> Apply
        </Button>
      </div>
      <div className="flex flex-wrap gap-1.5">
        <SelectedMemberBadges
          members={selectedMembers}
          label={label}
          onRemove={(id) => setSelectedIds((ids) => ids.filter((candidate) => candidate !== id))}
        />
      </div>
      <div className="flex gap-2">
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              void search();
            }
          }}
          placeholder="Search project members"
          aria-label={`Search ${label.toLowerCase()}`}
        />
        <Button
          size="icon"
          variant="outline"
          className="h-11 w-11 shrink-0 cursor-pointer sm:h-9 sm:w-9"
          aria-label={`Search ${label.toLowerCase()}`}
          disabled={searching}
          onClick={() => void search()}
        >
          <IconSearch className="h-4 w-4" />
        </Button>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
      {members.length > 0 && (
        <div className="max-h-44 space-y-1 overflow-y-auto rounded-md border p-1">
          {members.map((member) => (
            <label
              key={member.id}
              className="flex min-h-11 cursor-pointer items-center gap-2 rounded px-2 text-sm hover:bg-muted"
            >
              <Checkbox
                checked={selectedIds.includes(member.id)}
                onCheckedChange={() => setSelectedIds((ids) => toggleMemberId(ids, member.id))}
              />
              <span className="min-w-0 truncate">
                <strong>{member.name}</strong>{" "}
                <span className="text-muted-foreground">@{member.username}</span>
              </span>
            </label>
          ))}
        </div>
      )}
    </section>
  );
}
