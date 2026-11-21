import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { canAdmitTabletFile } from "@/lib/state/tablet-file-authority";
import type { OpenFileTab } from "@/lib/types/backend";
import { calculateHash } from "@/lib/utils/file-diff";

export function useCenterPanelGitChanges(sessionId: string | null): boolean | undefined {
  const gitStatus = useSessionGitStatus(sessionId);
  const { commits, loaded, loading } = useSessionCommits(sessionId);
  return useMemo(() => {
    if (Object.keys(gitStatus?.files ?? {}).length > 0 || commits.length > 0) return true;
    const ready =
      gitStatus &&
      gitStatus.status_state !== "loading" &&
      gitStatus.status_state !== "unavailable" &&
      gitStatus.files_complete !== false &&
      gitStatus.detail_state !== "pending" &&
      gitStatus.detail_state !== "unavailable";
    return ready && loaded && !loading ? false : undefined;
  }, [gitStatus, commits, loaded, loading]);
}

/** Ref and rendered tabs publish the same admission, including aliases restored between renders. */
export function useLiveFileTabs() {
  const current = useRef<OpenFileTab[]>([]);
  const [tabs, publish] = useState(current.current);
  const setTabs = useCallback<Dispatch<SetStateAction<OpenFileTab[]>>>((next) => {
    const admitted = typeof next === "function" ? next(current.current) : next;
    current.current = admitted;
    publish(admitted);
  }, []);
  return [tabs, setTabs, current] as const;
}

export function useTabletFileRequest(
  request: OpenFileTab | null,
  acknowledge: () => void,
  addFile: (file: OpenFileTab) => void,
) {
  useEffect(() => {
    if (!request) return;
    let current = true;
    queueMicrotask(async () => {
      if (!current) return;
      if (!canAdmitTabletFile(request)) {
        acknowledge();
        return;
      }
      const hash = request.originalHash || (await calculateHash(request.content));
      if (!current) return;
      if (canAdmitTabletFile(request))
        addFile({
          ...request,
          originalContent: request.originalContent || request.content,
          originalHash: hash,
          isDirty: request.isDirty ?? false,
        });
      acknowledge();
    });
    return () => {
      current = false;
    };
  }, [request, acknowledge, addFile]);
}
