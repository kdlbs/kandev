"use client";

import { useEffect, useRef, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import {
  listLinearLabels,
  listLinearStates,
  listLinearTeams,
  listLinearUsers,
} from "@/lib/api/domains/linear-api";
import type { LinearLabel, LinearTeam, LinearUser, LinearWorkflowState } from "@/lib/types/linear";

// useTeamsAndStates loads the team list once Linear is configured, plus the
// states, labels, and users for the currently-selected team. Each per-team
// dataset is cached so switching teams renders an empty list (or the cached
// list) without us having to setState in an effect — only the lookup
// expression changes.
export function useTeamsAndStates(teamKey: string) {
  const [teams, setTeams] = useState<LinearTeam[]>([]);
  const [statesByTeam, setStatesByTeam] = useState<Record<string, LinearWorkflowState[]>>({});
  const [labelsByTeam, setLabelsByTeam] = useState<Record<string, LinearLabel[]>>({});
  const [usersByTeam, setUsersByTeam] = useState<Record<string, LinearUser[]>>({});
  const fetchedTeams = useRef<Set<string>>(new Set());

  useEffect(() => {
    let cancelled = false;
    listLinearTeams()
      .then((res) => {
        if (!cancelled) setTeams(res.teams ?? []);
      })
      .catch(() => {
        if (!cancelled) setTeams([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!teamKey || fetchedTeams.current.has(teamKey)) return;
    // Mark in-flight so concurrent renders don't double-fetch; clear on
    // failure so a subsequent remount can retry instead of being stuck with
    // an empty cached entry.
    fetchedTeams.current.add(teamKey);
    let cancelled = false;
    let anyFailed = false;
    const markFailed = () => {
      anyFailed = true;
    };
    Promise.allSettled([
      listLinearStates(teamKey)
        .then((res) => {
          if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [teamKey]: res.states ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [teamKey]: [] }));
        }),
      listLinearLabels(teamKey)
        .then((res) => {
          if (!cancelled) setLabelsByTeam((prev) => ({ ...prev, [teamKey]: res.labels ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setLabelsByTeam((prev) => ({ ...prev, [teamKey]: [] }));
        }),
      listLinearUsers(teamKey)
        .then((res) => {
          if (!cancelled) setUsersByTeam((prev) => ({ ...prev, [teamKey]: res.users ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setUsersByTeam((prev) => ({ ...prev, [teamKey]: [] }));
        }),
    ]).finally(() => {
      // Clear the marker on failure OR cancellation so a remount can retry
      // — leaving it set would permanently strand the team's data as
      // whatever partial result the previous effect left behind.
      if (anyFailed || cancelled) fetchedTeams.current.delete(teamKey);
    });
    return () => {
      cancelled = true;
    };
  }, [teamKey]);

  const states = teamKey ? (statesByTeam[teamKey] ?? []) : [];
  const labels = teamKey ? (labelsByTeam[teamKey] ?? []) : [];
  const users = teamKey ? (usersByTeam[teamKey] ?? []) : [];
  const loadingStates = !!teamKey && statesByTeam[teamKey] === undefined;
  const loadingLabels = !!teamKey && labelsByTeam[teamKey] === undefined;
  const loadingUsers = !!teamKey && usersByTeam[teamKey] === undefined;
  return { teams, states, labels, users, loadingStates, loadingLabels, loadingUsers };
}

export function StateMultiSelect({
  states,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  states: LinearWorkflowState[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  if (disabled) {
    return (
      <p className="text-xs text-muted-foreground">
        Pick a team to choose specific workflow states.
      </p>
    );
  }
  if (loading) {
    return <p className="text-xs text-muted-foreground">Loading states…</p>;
  }
  if (states.length === 0) {
    return <p className="text-xs text-muted-foreground">No workflow states available.</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {states.map((s) => {
        const active = selected.includes(s.id);
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => onToggle(s.id)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant={active ? "default" : "outline"}>{s.name}</Badge>
          </button>
        );
      })}
    </div>
  );
}

export function LabelMultiSelect({
  labels,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  labels: LinearLabel[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  if (disabled) {
    return <p className="text-xs text-muted-foreground">Pick a team to choose specific labels.</p>;
  }
  if (loading) {
    return <p className="text-xs text-muted-foreground">Loading labels…</p>;
  }
  if (labels.length === 0) {
    return <p className="text-xs text-muted-foreground">No labels available for this team.</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {labels.map((l) => {
        const active = selected.includes(l.id);
        return (
          <button
            key={l.id}
            type="button"
            onClick={() => onToggle(l.id)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant={active ? "default" : "outline"}>{l.name}</Badge>
          </button>
        );
      })}
    </div>
  );
}
