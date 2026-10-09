"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { calculateHash } from "@/lib/utils/file-diff";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import type { OpenFileTab } from "@/lib/types/backend";
import type { FileChangeNotificationPayload, MarkdownFileMode } from "@/lib/types/workspace-files";

export type MobileFileSavedSnapshot = {
  path: string;
  repo?: string;
  sessionId: string | null;
  content: string;
  originalContent: string;
  originalHash: string;
};

export function getMobileFileIdentity(file: Pick<OpenFileTab, "path" | "repo">): string {
  return `${file.repo ?? ""}\u0000${file.path}`;
}

export function hasSelectedMobileFileChange(
  file: Pick<OpenFileTab, "path" | "repo">,
  changes: readonly { path: string; operation: string; repository_name?: string }[],
): boolean {
  return changes.some((change) => {
    if ((change.repository_name ?? "") !== (file.repo ?? "")) return false;
    return change.operation === "refresh" || change.path === file.path;
  });
}

export function reconcileMobileFileUpdate(
  current: OpenFileTab,
  remoteContent: string,
  remoteHash: string,
  isBinary?: boolean,
): OpenFileTab {
  if (current.isDirty && current.content !== remoteContent) {
    if (current.hasRemoteUpdate && current.remoteContent === remoteContent) return current;
    return {
      ...current,
      hasRemoteUpdate: true,
      remoteContent,
      remoteOriginalHash: remoteHash,
    };
  }
  return {
    ...current,
    content: remoteContent,
    originalContent: remoteContent,
    originalHash: remoteHash,
    isDirty: false,
    isBinary,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  };
}

export function useMobileFileSessionLifecycle(
  effectiveSessionId: string | null,
  setSelectedFile: Dispatch<SetStateAction<OpenFileTab | null>>,
  setSelectedFileMode: Dispatch<SetStateAction<MarkdownFileMode | undefined>>,
) {
  const [trackedSessionId, setTrackedSessionId] = useState<string | null>(effectiveSessionId);
  const latestRequestIdRef = useRef(0);
  const openFileAbortRef = useRef<AbortController | null>(null);

  if (trackedSessionId !== effectiveSessionId) {
    setTrackedSessionId(effectiveSessionId);
    setSelectedFile(null);
    setSelectedFileMode(undefined);
  }

  useLayoutEffect(() => {
    latestRequestIdRef.current += 1;
    openFileAbortRef.current?.abort();
    openFileAbortRef.current = null;
  }, [effectiveSessionId]);

  useEffect(
    () => () => {
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
    },
    [],
  );

  return { latestRequestIdRef, openFileAbortRef };
}

export function useMobileSelectedFileReload(
  selectedFile: OpenFileTab | null,
  setSelectedFile: Dispatch<SetStateAction<OpenFileTab | null>>,
) {
  return useCallback(() => {
    const current = selectedFile;
    if (!current?.hasRemoteUpdate || current.remoteContent === undefined) return;
    const remoteContent = current.remoteContent;
    void (
      current.remoteOriginalHash
        ? Promise.resolve(current.remoteOriginalHash)
        : calculateHash(remoteContent)
    ).then((remoteHash) => {
      setSelectedFile((latest) => {
        if (!latest || latest !== current) return latest;
        return {
          ...latest,
          content: remoteContent,
          originalContent: remoteContent,
          originalHash: remoteHash,
          isDirty: false,
          hasRemoteUpdate: false,
          remoteContent: undefined,
          remoteOriginalHash: undefined,
        };
      });
    });
  }, [selectedFile, setSelectedFile]);
}

export function useMobileSelectedFileWorkspaceSync(
  sessionId: string | null,
  selectedFileRef: MutableRefObject<OpenFileTab | null>,
  setSelectedFile: Dispatch<SetStateAction<OpenFileTab | null>>,
) {
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    const client = getWebSocketClient();
    if (!client || !sessionId || connectionStatus !== "connected") return;
    let requestVersion = 0;

    const handleFileChanges = (message: { payload: FileChangeNotificationPayload }) => {
      if (message.payload.session_id !== sessionId) return;
      const current = selectedFileRef.current;
      if (!current || !hasSelectedMobileFileChange(current, message.payload.changes)) return;
      const version = ++requestVersion;
      void requestFileContent(client, sessionId, current.path, current.repo)
        .then(async (response) => {
          const remoteHash = await calculateHash(response.content);
          if (version !== requestVersion) return;
          setSelectedFile((latest) => {
            if (!latest || getMobileFileIdentity(latest) !== getMobileFileIdentity(current)) {
              return latest;
            }
            return reconcileMobileFileUpdate(
              latest,
              response.content,
              remoteHash,
              response.is_binary,
            );
          });
        })
        .catch(() => {
          // Keep the current buffer when a notification arrives before the workspace is ready.
        });
    };

    const unsubscribe = client.on("session.workspace.file.changes", handleFileChanges);
    return () => {
      requestVersion += 1;
      unsubscribe();
    };
  }, [connectionStatus, selectedFileRef, sessionId, setSelectedFile]);
}

export function useSelectedMobileFileCallbacks(
  sessionId: string | null,
  setSelectedFile: Dispatch<SetStateAction<OpenFileTab | null>>,
  setSelectedFileMode: Dispatch<SetStateAction<MarkdownFileMode | undefined>>,
) {
  const handleSelectedFileChange = useCallback(
    (content: string) => {
      setSelectedFile((current) =>
        current ? { ...current, content, isDirty: content !== current.originalContent } : current,
      );
    },
    [setSelectedFile],
  );
  const handleSelectedFileSaved = useCallback(
    (snapshot: MobileFileSavedSnapshot) => {
      setSelectedFile((current) => {
        if (
          !current ||
          snapshot.sessionId !== sessionId ||
          getMobileFileIdentity(current) !== getMobileFileIdentity(snapshot)
        ) {
          return current;
        }
        const contentMatchesSavedSnapshot = current.content === snapshot.content;
        return {
          ...current,
          content: contentMatchesSavedSnapshot ? snapshot.content : current.content,
          originalContent: snapshot.originalContent,
          originalHash: snapshot.originalHash,
          isDirty: !contentMatchesSavedSnapshot,
          hasRemoteUpdate: contentMatchesSavedSnapshot ? false : current.hasRemoteUpdate,
          remoteContent: contentMatchesSavedSnapshot ? undefined : current.remoteContent,
          remoteOriginalHash: contentMatchesSavedSnapshot ? undefined : current.remoteOriginalHash,
        };
      });
    },
    [sessionId, setSelectedFile],
  );
  const handleSelectedFileModeChange = useCallback(
    (mode: MarkdownFileMode) => {
      setSelectedFileMode(mode);
      setSelectedFile((current) => (current ? { ...current, markdownMode: mode } : current));
    },
    [setSelectedFile, setSelectedFileMode],
  );
  return {
    handleSelectedFileChange,
    handleSelectedFileSaved,
    handleSelectedFileModeChange,
  };
}
