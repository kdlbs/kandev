'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconGitBranch, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { SettingsSection } from '@/components/settings/settings-section';
import { RepositoryCard } from '@/components/settings/repository-card';
import { generateUUID } from '@/lib/utils';
import {
  createRepositoryAction,
  createRepositoryScriptAction,
  deleteRepositoryAction,
  deleteRepositoryScriptAction,
  discoverRepositoriesAction,
  updateRepositoryAction,
  updateRepositoryScriptAction,
  validateRepositoryPathAction,
} from '@/app/actions/workspaces';
import type { LocalRepository, Repository, RepositoryScript, Workspace } from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type WorkspaceRepositoriesClientProps = {
  workspace: Workspace | null;
  repositories: RepositoryWithScripts[];
};

export function WorkspaceRepositoriesClient({
  workspace,
  repositories,
}: WorkspaceRepositoriesClientProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [repositoryItems, setRepositoryItems] = useState<RepositoryWithScripts[]>(repositories);
  const [savedRepositoryItems, setSavedRepositoryItems] = useState<RepositoryWithScripts[]>(repositories);
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [localRepoDialogOpen, setLocalRepoDialogOpen] = useState(false);
  const [remoteRepoDialogOpen, setRemoteRepoDialogOpen] = useState(false);
  const [repoSearch, setRepoSearch] = useState('');
  const [selectedRepoPath, setSelectedRepoPath] = useState<string | null>(null);
  const [manualRepoPath, setManualRepoPath] = useState('');
  const [manualValidation, setManualValidation] = useState<{
    status: 'idle' | 'loading' | 'success' | 'error';
    message?: string;
    isValid?: boolean;
    path?: string;
  }>({ status: 'idle' });
  const discoverRepositoriesRequest = useRequest(discoverRepositoriesAction);
  const validateRepositoryPathRequest = useRequest(validateRepositoryPathAction);

  const cloneRepository = (repo: RepositoryWithScripts): RepositoryWithScripts => ({
    ...repo,
    scripts: repo.scripts.map((script) => ({ ...script })),
  });

  const savedRepositoriesById = useMemo(() => {
    return new Map(savedRepositoryItems.map((repo) => [repo.id, repo]));
  }, [savedRepositoryItems]);

  const filteredRepositories = useMemo(() => {
    const query = repoSearch.trim().toLowerCase();
    if (!query) return discoveredRepositories;
    return discoveredRepositories.filter(
      (repo) => repo.name.toLowerCase().includes(query) || repo.path.toLowerCase().includes(query)
    );
  }, [discoveredRepositories, repoSearch]);

  const canSaveLocalRepo =
    Boolean(selectedRepoPath) || (manualValidation.status === 'success' && manualValidation.isValid);

  const isRepositoryDirty = (repo: RepositoryWithScripts) => {
    const saved = savedRepositoriesById.get(repo.id);
    if (!saved) return true;
    return (
      repo.name !== saved.name ||
      repo.source_type !== saved.source_type ||
      repo.local_path !== saved.local_path ||
      repo.provider !== saved.provider ||
      repo.provider_repo_id !== saved.provider_repo_id ||
      repo.provider_owner !== saved.provider_owner ||
      repo.provider_name !== saved.provider_name ||
      repo.default_branch !== saved.default_branch ||
      repo.worktree_branch_prefix !== saved.worktree_branch_prefix ||
      repo.setup_script !== saved.setup_script ||
      repo.cleanup_script !== saved.cleanup_script
    );
  };

  const areRepositoryScriptsDirty = (repo: RepositoryWithScripts) => {
    const saved = savedRepositoriesById.get(repo.id);
    if (!saved) return repo.scripts.length > 0;
    if (repo.scripts.length !== saved.scripts.length) return true;
    const savedScripts = new Map(saved.scripts.map((script) => [script.id, script]));
    for (const script of repo.scripts) {
      const savedScript = savedScripts.get(script.id);
      if (!savedScript) return true;
      if (
        script.name !== savedScript.name ||
        script.command !== savedScript.command ||
        script.position !== savedScript.position
      ) {
        return true;
      }
    }
    return false;
  };

  const handleUpdateRepository = (repoId: string, updates: Partial<Repository>) => {
    setRepositoryItems((prev) =>
      prev.map((repo) => (repo.id === repoId ? { ...repo, ...updates } : repo))
    );
  };

  const handleDiscoverRepositories = async () => {
    if (!workspace) return;
    try {
      const result = await discoverRepositoriesRequest.run(workspace.id);
      setDiscoveredRepositories(result.repositories);
    } catch (error) {
      toast({
        title: 'Failed to discover repositories',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const openLocalRepositoryDialog = async () => {
    setLocalRepoDialogOpen(true);
    setRepoSearch('');
    setSelectedRepoPath(null);
    setManualRepoPath('');
    setManualValidation({ status: 'idle' });
    await handleDiscoverRepositories();
  };

  const handleValidateManualPath = async () => {
    if (!workspace || !manualRepoPath.trim()) return;
    setManualValidation({ status: 'loading' });
    try {
      const result = await validateRepositoryPathRequest.run(workspace.id, manualRepoPath.trim());
      if (result.allowed && result.exists && result.is_git) {
        setManualValidation({
          status: 'success',
          isValid: true,
          message: 'Valid git repository',
          path: result.path,
        });
      } else {
        setManualValidation({
          status: 'error',
          isValid: false,
          message: result.message || 'Invalid repository path',
          path: result.path,
        });
      }
    } catch (error) {
      setManualValidation({
        status: 'error',
        isValid: false,
        message: error instanceof Error ? error.message : 'Request failed',
      });
    }
  };

  const handleConfirmLocalRepository = () => {
    if (!workspace) return;
    const selectedRepo = discoveredRepositories.find((repo) => repo.path === selectedRepoPath);
    const path = selectedRepo?.path || manualValidation.path || manualRepoPath.trim();
    if (!path) return;
    const name = selectedRepo?.name || path.split('/').filter(Boolean).slice(-1)[0] || 'New Repository';
    const draftId = `temp-repo-${generateUUID()}`;
    const draftRepo: RepositoryWithScripts = {
      id: draftId,
      workspace_id: workspace.id,
      name,
      source_type: 'local',
      local_path: path,
      provider: '',
      provider_repo_id: '',
      provider_owner: '',
      provider_name: '',
      default_branch: selectedRepo?.default_branch || '',
      worktree_branch_prefix: 'feature/',
      setup_script: '',
      cleanup_script: '',
      created_at: '',
      updated_at: '',
      scripts: [],
    };
    setRepositoryItems((prev) => [draftRepo, ...prev]);
    setLocalRepoDialogOpen(false);
  };

  const handleAddRepositoryScript = (repoId: string) => {
    const script: RepositoryScript = {
      id: `temp-script-${generateUUID()}`,
      repository_id: repoId,
      name: '',
      command: '',
      position: repositoryItems.find((repo) => repo.id === repoId)?.scripts.length ?? 0,
      created_at: '',
      updated_at: '',
    };
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId ? { ...repo, scripts: [...repo.scripts, script] } : repo
      )
    );
  };

  const handleUpdateRepositoryScript = (
    repoId: string,
    scriptId: string,
    updates: Partial<RepositoryScript>
  ) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? {
              ...repo,
              scripts: repo.scripts.map((script) =>
                script.id === scriptId ? { ...script, ...updates } : script
              ),
            }
          : repo
      )
    );
  };

  const handleDeleteRepositoryScript = (repoId: string, scriptId: string) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? { ...repo, scripts: repo.scripts.filter((script) => script.id !== scriptId) }
          : repo
      )
    );
  };

  const handleSaveRepository = async (repoId: string) => {
    const repo = repositoryItems.find((item) => item.id === repoId);
    if (!repo) return;
    if (repoId.startsWith('temp-repo-')) {
      const created = await createRepositoryAction({
        workspace_id: workspace?.id ?? repo.workspace_id,
        name: repo.name.trim() || 'New Repository',
        source_type: repo.source_type || 'local',
        local_path: repo.local_path,
        provider: repo.provider,
        provider_repo_id: repo.provider_repo_id,
        provider_owner: repo.provider_owner,
        provider_name: repo.provider_name,
        default_branch: repo.default_branch,
        worktree_branch_prefix: repo.worktree_branch_prefix,
        setup_script: repo.setup_script,
        cleanup_script: repo.cleanup_script,
      });
      const scripts = await Promise.all(
        repo.scripts.map((script, index) =>
          createRepositoryScriptAction({
            repository_id: created.id,
            name: script.name.trim() || 'New Script',
            command: script.command.trim() || 'echo ""',
            position: script.position ?? index,
          })
        )
      );
      const nextRepo: RepositoryWithScripts = { ...created, scripts };
      setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
      setSavedRepositoryItems((prev) => [cloneRepository(nextRepo), ...prev]);
      return;
    }

    const updated = await updateRepositoryAction(repoId, {
      name: repo.name,
      source_type: repo.source_type,
      local_path: repo.local_path,
      provider: repo.provider,
      provider_repo_id: repo.provider_repo_id,
      provider_owner: repo.provider_owner,
      provider_name: repo.provider_name,
      default_branch: repo.default_branch,
      worktree_branch_prefix: repo.worktree_branch_prefix,
      setup_script: repo.setup_script,
      cleanup_script: repo.cleanup_script,
    });

    const saved = savedRepositoriesById.get(repoId);
    const savedScripts = saved ? saved.scripts : [];
    const currentScriptIds = new Set(repo.scripts.map((script) => script.id));

    await Promise.all(
      savedScripts
        .filter((script) => !currentScriptIds.has(script.id))
        .map((script) => deleteRepositoryScriptAction(script.id))
    );

    const nextScripts = await Promise.all(
      repo.scripts.map((script, index) => {
        if (script.id.startsWith('temp-script-')) {
          return createRepositoryScriptAction({
            repository_id: repoId,
            name: script.name.trim() || 'New Script',
            command: script.command.trim() || 'echo ""',
            position: script.position ?? index,
          });
        }
        return updateRepositoryScriptAction(script.id, {
          name: script.name,
          command: script.command,
          position: script.position ?? index,
        });
      })
    );

    const nextRepo: RepositoryWithScripts = { ...updated, scripts: nextScripts };
    setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
    setSavedRepositoryItems((prev) =>
      prev.some((item) => item.id === repoId)
        ? prev.map((item) => (item.id === repoId ? cloneRepository(nextRepo) : item))
        : [...prev, cloneRepository(nextRepo)]
    );
  };

  const handleDeleteRepository = async (repoId: string) => {
    if (repoId.startsWith('temp-repo-')) {
      setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
      return;
    }
    await deleteRepositoryAction(repoId);
    setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
    setSavedRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
  };

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/workspace')}>
              Back to Workspaces
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage repositories connected to this workspace.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Repositories"
        description="Repositories in this workspace"
        action={
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={openLocalRepositoryDialog}>
              Local Repository
            </Button>
            <Button size="sm" variant="outline" onClick={() => setRemoteRepoDialogOpen(true)}>
              Remote Repository
            </Button>
          </div>
        }
      >
        <div className="grid gap-3">
          {repositoryItems.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              isRepositoryDirty={isRepositoryDirty(repo)}
              areScriptsDirty={areRepositoryScriptsDirty(repo)}
              onUpdate={handleUpdateRepository}
              onAddScript={handleAddRepositoryScript}
              onUpdateScript={handleUpdateRepositoryScript}
              onDeleteScript={handleDeleteRepositoryScript}
              onSave={handleSaveRepository}
              onDelete={handleDeleteRepository}
            />
          ))}
        </div>
      </SettingsSection>

      <Dialog open={localRepoDialogOpen} onOpenChange={setLocalRepoDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Add Local Repository</DialogTitle>
            <DialogDescription>
              Select a discovered repository or provide an absolute path to validate.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Discovered repositories</Label>
              <Input
                placeholder="Filter repositories..."
                value={repoSearch}
                onChange={(e) => setRepoSearch(e.target.value)}
              />
              <div className="max-h-56 overflow-auto rounded-md border border-border">
                {discoverRepositoriesRequest.isLoading ? (
                  <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
                    <IconLoader2 className="h-4 w-4 animate-spin" />
                    Scanning repositories...
                  </div>
                ) : filteredRepositories.length === 0 ? (
                  <div className="p-3 text-sm text-muted-foreground">No repositories found.</div>
                ) : (
                  filteredRepositories.map((repo) => (
                    <button
                      key={repo.path}
                      type="button"
                      className={`flex w-full flex-col px-3 py-2 text-left text-sm hover:bg-muted ${
                        selectedRepoPath === repo.path ? 'bg-muted' : ''
                      }`}
                      onClick={() => {
                        setSelectedRepoPath(repo.path);
                        setManualRepoPath('');
                        setManualValidation({ status: 'idle' });
                      }}
                    >
                      <span className="font-medium">{repo.name}</span>
                      <span className="text-xs text-muted-foreground">{repo.path}</span>
                    </button>
                  ))
                )}
              </div>
            </div>

            <div className="space-y-2">
              <Label>Manual path</Label>
              <div className="flex items-center gap-2">
                <Input
                  placeholder="/absolute/path/to/repository"
                  value={manualRepoPath}
                  onChange={(e) => {
                    setManualRepoPath(e.target.value);
                    setSelectedRepoPath(null);
                    setManualValidation({ status: 'idle' });
                  }}
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleValidateManualPath}
                  disabled={!manualRepoPath.trim() || validateRepositoryPathRequest.isLoading}
                >
                  {validateRepositoryPathRequest.isLoading ? 'Checking...' : 'Validate'}
                </Button>
              </div>
              {manualValidation.status === 'error' && (
                <p className="text-xs text-destructive">{manualValidation.message}</p>
              )}
              {manualValidation.status === 'success' && (
                <p className="text-xs text-emerald-500">{manualValidation.message}</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setLocalRepoDialogOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={handleConfirmLocalRepository} disabled={!canSaveLocalRepo}>
              Use Repository
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={remoteRepoDialogOpen} onOpenChange={setRemoteRepoDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Remote Repository</DialogTitle>
            <DialogDescription>Remote providers are coming soon.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              GitHub (coming soon)
            </Button>
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              GitLab (coming soon)
            </Button>
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              Bitbucket (coming soon)
            </Button>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRemoteRepoDialogOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
