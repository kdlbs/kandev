"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import { useCommentsStore, type PRFeedbackComment } from "@/lib/state/slices/comments";
import { useToast } from "@/components/toast-provider";
import { getMRCommits, getMRFeedback, getMRFiles } from "@/lib/api/domains/gitlab-api";
import type { GitLabMRCommit, GitLabMRFeedback, GitLabMRFile } from "@/lib/types/gitlab";

type State = {
  feedback: GitLabMRFeedback | null;
  files: GitLabMRFile[];
  commits: GitLabMRCommit[];
  loading: boolean;
  error: string | null;
  revision: number;
  identityKey: string | null;
};

type Action =
  | { type: "refresh" }
  | { type: "clear" }
  | { type: "loading"; identityKey: string }
  | {
      type: "loaded";
      feedback?: GitLabMRFeedback;
      files?: GitLabMRFile[];
      commits?: GitLabMRCommit[];
      error: string | null;
    }
  | { type: "failed"; error: string };

const initialState: State = {
  feedback: null,
  files: [],
  commits: [],
  loading: false,
  error: null,
  revision: 0,
  identityKey: null,
};

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "refresh":
      return { ...state, revision: state.revision + 1 };
    case "clear":
      return { ...initialState, revision: state.revision };
    case "loading":
      return state.identityKey === action.identityKey
        ? { ...state, loading: true, error: null }
        : {
            ...initialState,
            revision: state.revision,
            loading: true,
            identityKey: action.identityKey,
          };
    case "loaded":
      return {
        ...state,
        feedback: action.feedback ?? state.feedback,
        files: action.files ?? state.files,
        commits: action.commits ?? state.commits,
        loading: false,
        error: action.error,
      };
    case "failed":
      return { ...state, loading: false, error: action.error };
  }
}

export function useMRFeedback(
  workspaceId: string | null,
  project: string | null,
  iid: number | null,
  host?: string | null,
) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const requestGeneration = useRef(0);
  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);

  useEffect(() => {
    if (!workspaceId || !project || !iid) {
      requestGeneration.current += 1;
      dispatch({ type: "clear" });
      return;
    }
    const generation = ++requestGeneration.current;
    const identity = { workspaceId, project, iid, host: host ?? undefined };
    const identityKey = `${workspaceId}\0${host ?? ""}\0${project}\0${iid}`;
    dispatch({ type: "loading", identityKey });
    Promise.allSettled([getMRFeedback(identity), getMRFiles(identity), getMRCommits(identity)])
      .then(([feedback, files, commits]) => {
        if (requestGeneration.current === generation) {
          const errors = [feedback, files, commits]
            .filter((result): result is PromiseRejectedResult => result.status === "rejected")
            .map((result) =>
              result.reason instanceof Error
                ? result.reason.message
                : "GitLab rejected the request",
            );
          dispatch({
            type: "loaded",
            feedback: feedback.status === "fulfilled" ? feedback.value : undefined,
            files: files.status === "fulfilled" ? (files.value.files ?? []) : undefined,
            commits: commits.status === "fulfilled" ? (commits.value.commits ?? []) : undefined,
            error: errors.length > 0 ? errors.join("; ") : null,
          });
        }
      })
      .catch((error: unknown) => {
        if (requestGeneration.current === generation) {
          dispatch({
            type: "failed",
            error: error instanceof Error ? error.message : "Failed to load merge request",
          });
        }
      });
    return () => {
      if (requestGeneration.current === generation) requestGeneration.current += 1;
    };
  }, [workspaceId, project, iid, host, state.revision]);

  return { ...state, refresh };
}

export function useAddMRFeedbackAsContext(sessionId: string, projectPath: string, mrIid: number) {
  const addComment = useCommentsStore((state) => state.addComment);
  const { toast } = useToast();
  return useCallback(
    (content: string) => {
      const comment: PRFeedbackComment = {
        id: `mr-feedback-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        sessionId,
        text: content,
        createdAt: new Date().toISOString(),
        status: "pending",
        source: "pr-feedback",
        provider: "gitlab",
        prNumber: mrIid,
        mrIid,
        projectPath,
        feedbackType: "comment",
        content,
      };
      addComment(comment);
      toast({ description: "Added merge request feedback to task context", variant: "success" });
    },
    [addComment, mrIid, projectPath, sessionId, toast],
  );
}
