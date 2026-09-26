"use client";

import { DiscoverRepoDialog } from "@/app/settings/workspace/workspace-repositories-dialog";
import type { useWorkspaceRepositoriesPage } from "./workspace-repositories-client";

/**
 * The discover/add-local-repository dialog for the workspace Repositories page.
 *
 * It takes the page's whole state object rather than fifteen props: every field it
 * reads belongs to that one hook, and passing them individually made the page
 * component exceed its length cap once the repository-sets section joined it.
 */
export function AddLocalRepositoryDialog({
  state,
}: {
  state: ReturnType<typeof useWorkspaceRepositoriesPage>;
}) {
  return (
    <DiscoverRepoDialog
      open={state.localRepoDialogOpen}
      onOpenChange={state.setLocalRepoDialogOpen}
      isLoading={state.isDiscovering}
      filteredRepositories={state.filteredRepositories}
      repoSearch={state.repoSearch}
      onRepoSearchChange={state.setRepoSearch}
      selectedRepoPath={state.selectedRepoPath}
      onSelectRepoPath={state.handleSelectRepoPath}
      manualRepoPath={state.manualRepoPath}
      onManualRepoPathChange={state.handleManualRepoPathChange}
      manualValidation={state.manualValidation}
      onValidateManualPath={state.handleValidateManualPath}
      isValidating={state.isValidating}
      canSave={state.canSave}
      onConfirm={state.handleConfirmLocalRepository}
      desktopRuntime={state.desktopRuntime}
      workspaceId={state.workspaceId}
      onRefreshDiscovery={state.onRefreshDiscovery}
    />
  );
}
