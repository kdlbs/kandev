import type { FormEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useAgentProjectMutations } from "@/hooks/domains/agent-projects/use-agent-projects";
import { generateUUID } from "@/lib/utils";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { draftFor, useProjectFormData } from "./agent-project-form-fields";
import type { ProjectDraft } from "./agent-project-form-fields";

function isProjectFormReady(
  draft: ProjectDraft,
  project: AgentProject | undefined,
  formData: ReturnType<typeof useProjectFormData>,
) {
  const profileIds = [draft.coordinatorProfileId, draft.economyProfileId, draft.frontierProfileId];
  const repositoryIdsAvailable = draft.repositoryIds.every((id) =>
    formData.repositories.some((repository) => repository.id === id),
  );
  const profileIdsAvailable = profileIds.every((id) =>
    formData.profiles.some((profile) => profile.id === id),
  );
  return (
    draft.name.trim().length > 0 &&
    draft.repositoryIds.length > 0 &&
    draft.repositoryIds.includes(draft.primaryRepositoryId) &&
    profileIds.every(Boolean) &&
    (Boolean(project) || formData.executorReady) &&
    formData.repositoriesLoaded &&
    repositoryIdsAvailable &&
    profileIdsAvailable
  );
}

async function persistProjectDraft({
  mutations,
  workspaceId,
  project,
  draft,
  applyDefaultExecutor,
  requestKey,
}: {
  mutations: ReturnType<typeof useAgentProjectMutations>;
  workspaceId: string;
  project: AgentProject | undefined;
  draft: ProjectDraft;
  applyDefaultExecutor: boolean;
  requestKey: string;
}) {
  const payload = {
    name: draft.name.trim(),
    repositoryIds: draft.repositoryIds,
    primaryRepositoryId: draft.primaryRepositoryId,
    coordinatorProfileId: draft.coordinatorProfileId,
    economyProfileId: draft.economyProfileId,
    frontierProfileId: draft.frontierProfileId,
  };
  if (project) {
    await mutations.update(workspaceId, project.id, {
      ...payload,
      revision: project.revision,
      applyWorkspaceDefaultExecutor: applyDefaultExecutor || undefined,
    });
    return;
  }
  await mutations.create(workspaceId, { ...payload, requestKey });
}

export function useAgentProjectFormController(
  open: boolean,
  workspaceId: string,
  project: AgentProject | undefined,
  formData: ReturnType<typeof useProjectFormData>,
  onOpenChange: (open: boolean) => void,
) {
  const mutations = useAgentProjectMutations();
  const [draft, setDraft] = useState<ProjectDraft>(() => draftFor(project));
  const [applyDefaultExecutor, setApplyDefaultExecutor] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const createRequestKey = useRef<string | null>(null);

  useEffect(() => {
    if (!open) {
      createRequestKey.current = null;
      return;
    }
    createRequestKey.current = project ? null : (createRequestKey.current ?? generateUUID());
    setDraft(draftFor(project));
    setApplyDefaultExecutor(false);
    setError(null);
  }, [open, project]);

  const ready = useMemo(
    () => isProjectFormReady(draft, project, formData),
    [draft, formData, project],
  );
  const updateDraft = (patch: Partial<ProjectDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));
  const toggleRepository = (repositoryId: string) => {
    const repositoryIds = draft.repositoryIds.includes(repositoryId)
      ? draft.repositoryIds.filter((id) => id !== repositoryId)
      : [...draft.repositoryIds, repositoryId];
    const primaryRepositoryId = repositoryIds.includes(draft.primaryRepositoryId)
      ? draft.primaryRepositoryId
      : (repositoryIds[0] ?? "");
    updateDraft({ repositoryIds, primaryRepositoryId });
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!ready || saving) return;
    setSaving(true);
    setError(null);
    try {
      const requestKey = (createRequestKey.current ??= generateUUID());
      await persistProjectDraft({
        mutations,
        workspaceId,
        project,
        draft,
        applyDefaultExecutor,
        requestKey,
      });
      onOpenChange(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setSaving(false);
    }
  };

  return {
    draft,
    updateDraft,
    toggleRepository,
    applyDefaultExecutor,
    setApplyDefaultExecutor,
    ready,
    error,
    saving,
    submit,
  };
}
