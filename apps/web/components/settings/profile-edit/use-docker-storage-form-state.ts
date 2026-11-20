import { useCallback, useEffect, useState } from "react";

type DockerStorageDraft = {
  value: string;
  dirty: boolean;
};

function refreshDockerStorageDraft(
  current: DockerStorageDraft,
  savedValue: string,
): DockerStorageDraft {
  if (current.dirty && current.value.trim() !== savedValue) return current;
  return { value: savedValue, dirty: false };
}

export function useDockerStorageFormState(config?: Record<string, string>) {
  const baselineDockerTmpSize = config?.docker_tmp_size ?? "";
  const baselineDockerLogMaxSize = config?.docker_log_max_size ?? "";
  const baselineDockerLogMaxFiles = config?.docker_log_max_files ?? "";
  const [dockerTmpSizeDraft, setDockerTmpSizeDraft] = useState<DockerStorageDraft>(() => ({
    value: baselineDockerTmpSize,
    dirty: false,
  }));
  const [dockerLogMaxSizeDraft, setDockerLogMaxSizeDraft] = useState<DockerStorageDraft>(() => ({
    value: baselineDockerLogMaxSize,
    dirty: false,
  }));
  const [dockerLogMaxFilesDraft, setDockerLogMaxFilesDraft] = useState<DockerStorageDraft>(() => ({
    value: baselineDockerLogMaxFiles,
    dirty: false,
  }));

  useEffect(() => {
    setDockerTmpSizeDraft((current) => refreshDockerStorageDraft(current, baselineDockerTmpSize));
    setDockerLogMaxSizeDraft((current) =>
      refreshDockerStorageDraft(current, baselineDockerLogMaxSize),
    );
    setDockerLogMaxFilesDraft((current) =>
      refreshDockerStorageDraft(current, baselineDockerLogMaxFiles),
    );
  }, [baselineDockerLogMaxFiles, baselineDockerLogMaxSize, baselineDockerTmpSize]);

  const setDockerTmpSize = useCallback((value: string) => {
    setDockerTmpSizeDraft({ value, dirty: true });
  }, []);
  const setDockerLogMaxSize = useCallback((value: string) => {
    setDockerLogMaxSizeDraft({ value, dirty: true });
  }, []);
  const setDockerLogMaxFiles = useCallback((value: string) => {
    setDockerLogMaxFilesDraft({ value, dirty: true });
  }, []);

  const resetDockerStorage = useCallback(() => {
    setDockerTmpSizeDraft({ value: baselineDockerTmpSize, dirty: false });
    setDockerLogMaxSizeDraft({ value: baselineDockerLogMaxSize, dirty: false });
    setDockerLogMaxFilesDraft({ value: baselineDockerLogMaxFiles, dirty: false });
  }, [baselineDockerLogMaxFiles, baselineDockerLogMaxSize, baselineDockerTmpSize]);

  return {
    dockerTmpSize: dockerTmpSizeDraft.value,
    dockerTmpSizeDirty: dockerTmpSizeDraft.dirty,
    setDockerTmpSize,
    baselineDockerTmpSize,
    dockerLogMaxSize: dockerLogMaxSizeDraft.value,
    dockerLogMaxSizeDirty: dockerLogMaxSizeDraft.dirty,
    setDockerLogMaxSize,
    baselineDockerLogMaxSize,
    dockerLogMaxFiles: dockerLogMaxFilesDraft.value,
    dockerLogMaxFilesDirty: dockerLogMaxFilesDraft.dirty,
    setDockerLogMaxFiles,
    baselineDockerLogMaxFiles,
    resetDockerStorage,
  };
}
