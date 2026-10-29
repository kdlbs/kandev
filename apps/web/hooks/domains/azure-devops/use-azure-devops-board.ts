"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAzureDevOpsMutationOwner } from "./use-azure-devops-mutation-owner";
import {
  getAzureDevOpsBoardSnapshot,
  listAzureDevOpsBoards,
  listAzureDevOpsTeams,
  updateAzureDevOpsBoardWorkItem,
} from "@/lib/api/domains/azure-devops-api";
import type {
  AzureDevOpsBoardSnapshot,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsBoardWorkItemUpdate,
  AzureDevOpsBoardReference,
  AzureDevOpsTeam,
} from "@/lib/types/azure-devops";

type BoardWorkItemChanges = Omit<AzureDevOpsBoardWorkItemUpdate, "revision">;

function errorMessage(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback;
}

function replaceBoardItem(
  snapshot: AzureDevOpsBoardSnapshot | null,
  id: number,
  item: AzureDevOpsBoardWorkItem,
): AzureDevOpsBoardSnapshot | null {
  if (!snapshot) return snapshot;
  return {
    ...snapshot,
    items: snapshot.items.map((candidate) =>
      candidate.id === id && candidate.revision <= item.revision ? item : candidate,
    ),
  };
}

function useBoardDiscovery(
  workspaceId: string | undefined,
  projectId: string,
  preference?: { teamId?: string; boardId?: string },
) {
  const preferredTeamId = preference?.teamId;
  const preferredBoardId = preference?.boardId;
  const preferredTeamIdRef = useRef(preferredTeamId);
  const preferredBoardIdRef = useRef(preferredBoardId);
  preferredTeamIdRef.current = preferredTeamId;
  preferredBoardIdRef.current = preferredBoardId;
  const [teams, setTeams] = useState<AzureDevOpsTeam[]>([]);
  const [boards, setBoards] = useState<AzureDevOpsBoardReference[]>([]);
  const [teamId, setTeamId] = useState("");
  const [boardId, setBoardId] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId || !projectId) {
      setTeams([]);
      setTeamId("");
      return;
    }
    let cancelled = false;
    setBoards([]);
    setTeamId("");
    setBoardId("");
    setError(null);
    listAzureDevOpsTeams(workspaceId, projectId)
      .then((result) => {
        if (!cancelled) {
          setTeams(result.teams);
          const rememberedTeamID = preferredTeamIdRef.current;
          setTeamId(
            result.teams.some((team) => team.id === rememberedTeamID)
              ? (rememberedTeamID ?? "")
              : (result.teams[0]?.id ?? ""),
          );
        }
      })
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load teams")));
    return () => {
      cancelled = true;
    };
  }, [projectId, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !projectId || !teamId) {
      setBoards([]);
      setBoardId("");
      return;
    }
    let cancelled = false;
    setBoardId("");
    setError(null);
    listAzureDevOpsBoards(workspaceId, projectId, teamId)
      .then((result) => {
        if (!cancelled) {
          setBoards(result.boards);
          const rememberedBoardID = preferredBoardIdRef.current;
          setBoardId(
            result.boards.some((board) => board.id === rememberedBoardID)
              ? (rememberedBoardID ?? "")
              : (result.boards[0]?.id ?? ""),
          );
        }
      })
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load boards")));
    return () => {
      cancelled = true;
    };
  }, [projectId, teamId, workspaceId]);

  useEffect(() => {
    if (preferredTeamId && teams.some((team) => team.id === preferredTeamId)) {
      setTeamId(preferredTeamId);
    }
  }, [preferredTeamId, teams]);

  useEffect(() => {
    if (preferredBoardId && boards.some((board) => board.id === preferredBoardId)) {
      setBoardId(preferredBoardId);
    }
  }, [boards, preferredBoardId]);

  return { teams, boards, teamId, setTeamId, boardId, setBoardId, error };
}

function useBoardSnapshot(
  workspaceId: string | undefined,
  projectId: string,
  teamId: string,
  boardId: string,
) {
  const [snapshot, setSnapshot] = useState<AzureDevOpsBoardSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!workspaceId || !projectId || !teamId || !boardId) return;
    setLoading(true);
    setError(null);
    getAzureDevOpsBoardSnapshot(workspaceId, projectId, teamId, boardId)
      .then((next) => !cancelled && setSnapshot(next))
      .catch((cause) => !cancelled && setError(errorMessage(cause, "Unable to load board")))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [boardId, projectId, teamId, workspaceId]);

  return { snapshot, loading, error, setSnapshot, setError };
}

export function useAzureDevOpsBoard(
  workspaceId: string | undefined,
  projectId: string,
  preference?: { teamId?: string; boardId?: string },
) {
  const discovery = useBoardDiscovery(workspaceId, projectId, preference);
  const { teamId, boardId } = discovery;
  const beginMutation = useAzureDevOpsMutationOwner(
    JSON.stringify([workspaceId, projectId, teamId, boardId]),
  );
  const { snapshot, loading, error, setSnapshot, setError } = useBoardSnapshot(
    workspaceId,
    projectId,
    teamId,
    boardId,
  );

  const updateItem = useCallback(
    async (
      item: AzureDevOpsBoardWorkItem,
      values: BoardWorkItemChanges,
    ): Promise<AzureDevOpsBoardWorkItem | undefined> => {
      const isCurrent = beginMutation();
      if (!workspaceId || !teamId || !boardId || !isCurrent()) return undefined;
      setError(null);
      const optimistic = { ...item, ...values };
      setSnapshot((current) => replaceBoardItem(current, item.id, optimistic));
      try {
        const updated = await updateAzureDevOpsBoardWorkItem(
          workspaceId,
          projectId,
          teamId,
          boardId,
          item.id,
          { revision: item.revision, ...values },
        );
        if (!isCurrent()) return undefined;
        setSnapshot((current) =>
          isCurrent() ? replaceBoardItem(current, item.id, updated) : current,
        );
        return updated;
      } catch (cause) {
        if (!isCurrent()) return undefined;
        setSnapshot((current) => {
          if (!current || !isCurrent()) return current;
          return {
            ...current,
            items: current.items.map((candidate) => (candidate === optimistic ? item : candidate)),
          };
        });
        setError(errorMessage(cause, "Unable to update work item"));
        throw cause;
      }
    },
    [beginMutation, boardId, projectId, setError, setSnapshot, teamId, workspaceId],
  );

  const moveItem = useCallback(
    async (
      item: AzureDevOpsBoardWorkItem,
      columnId: string,
      columnDone = item.columnDone,
    ): Promise<AzureDevOpsBoardWorkItem | undefined> => {
      if (item.columnId !== columnId || item.columnDone !== columnDone) {
        return updateItem(item, { columnId, columnDone });
      }
      return item;
    },
    [updateItem],
  );

  const updateAssignee = useCallback(
    (item: AzureDevOpsBoardWorkItem, assigneeAction: "assign_current_user" | "unassign") =>
      updateItem(item, { assigneeAction }),
    [updateItem],
  );

  const mergeItem = useCallback(
    (item: AzureDevOpsBoardWorkItem) => {
      const isCurrent = beginMutation();
      if (!isCurrent()) return;
      setSnapshot((current) => (isCurrent() ? replaceBoardItem(current, item.id, item) : current));
    },
    [beginMutation, setSnapshot],
  );

  return {
    teams: discovery.teams,
    boards: discovery.boards,
    teamId,
    setTeamId: discovery.setTeamId,
    boardId,
    setBoardId: discovery.setBoardId,
    snapshot,
    loading,
    error: discovery.error ?? error,
    moveItem,
    updateAssignee,
    mergeItem,
  };
}
