'use client';

import { useCallback, useMemo } from 'react';
import {
  IconPlayerStop,
  IconFileTextSpark,
  IconGitCommit,
  IconArrowUp,
  IconArrowDown,
  IconGitPullRequest,
  IconGitBranch,
  IconGitMerge,
  IconBrowser,
  IconTerminal2,
  IconFileText,
  IconFileDiff,
  IconFilePlus,
} from '@tabler/icons-react';
import { useRegisterCommands } from '@/hooks/use-register-commands';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useGitWithFeedback } from '@/hooks/use-git-with-feedback';
import { usePanelActions } from '@/hooks/use-panel-actions';
import { useVcsDialogs } from '@/components/vcs/vcs-dialogs';
import { getWebSocketClient } from '@/lib/ws/connection';
import { createFile } from '@/lib/ws/workspace-files';
import { useDockviewStore } from '@/lib/state/dockview-store';
import type { CommandItem } from '@/lib/commands/types';

type SessionCommandsProps = {
  sessionId: string | null;
  baseBranch?: string;
  isAgentRunning?: boolean;
  hasWorktree?: boolean;
  isPassthrough?: boolean;
};

export function SessionCommands({ sessionId, baseBranch, isAgentRunning, hasWorktree, isPassthrough }: SessionCommandsProps) {
  const git = useGitOperations(sessionId);
  const panels = usePanelActions();
  const { openCommitDialog, openPRDialog } = useVcsDialogs();

  const cancelTurn = useCallback(async () => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request('agent.cancel', { session_id: sessionId }, 15000);
    } catch (error) {
      console.error('Failed to cancel agent turn:', error);
    }
  }, [sessionId]);

  const gitWithFeedback = useGitWithFeedback();

  const runGitWithFeedback = useCallback(
    async (
      operation: () => Promise<{ success: boolean; output: string; error?: string }>,
      operationName: string
    ) => {
      panels.addChanges();
      await gitWithFeedback(operation, operationName);
    },
    [panels, gitWithFeedback]
  );

  const commands = useMemo<CommandItem[]>(() => {
    if (!sessionId) return [];

    const PAGE_PRIORITY = 0;
    const items: CommandItem[] = [];

    // Session — conditional
    if (isAgentRunning) {
      items.push({
        id: 'session-cancel',
        label: 'Cancel Turn',
        group: 'Session',
        icon: <IconPlayerStop className="size-3.5" />,
        keywords: ['cancel', 'stop', 'turn'],
        action: cancelTurn,
      });
    }

    if (!isPassthrough) {
      items.push({
        id: 'session-plan-mode',
        label: 'Toggle Plan Mode',
        group: 'Session',
        icon: <IconFileTextSpark className="size-3.5" />,
        keywords: ['plan', 'mode', 'toggle'],
        action: () => panels.addPlan(),
      });
    }

    // Git — only when worktree is set up
    if (hasWorktree) {
      items.push(
        {
          id: 'git-commit',
          label: 'Commit Changes',
          group: 'Git',
          icon: <IconGitCommit className="size-3.5" />,
          keywords: ['commit', 'git', 'save'],
          action: openCommitDialog,
        },
        {
          id: 'git-push',
          label: 'Push',
          group: 'Git',
          icon: <IconArrowUp className="size-3.5" />,
          keywords: ['push', 'git', 'upload'],
          action: () => runGitWithFeedback(() => git.push(), 'Push'),
        },
        {
          id: 'git-pull',
          label: 'Pull',
          group: 'Git',
          icon: <IconArrowDown className="size-3.5" />,
          keywords: ['pull', 'git', 'download', 'fetch'],
          action: () => runGitWithFeedback(() => git.pull(), 'Pull'),
        },
        {
          id: 'git-create-pr',
          label: 'Create PR',
          group: 'Git',
          icon: <IconGitPullRequest className="size-3.5" />,
          keywords: ['pull request', 'pr', 'git'],
          action: openPRDialog,
        },
        {
          id: 'git-rebase',
          label: 'Rebase',
          group: 'Git',
          icon: <IconGitBranch className="size-3.5" />,
          keywords: ['rebase', 'git', 'branch'],
          action: () => {
            const target = baseBranch?.replace(/^origin\//, '') || 'main';
            return runGitWithFeedback(() => git.rebase(target), 'Rebase');
          },
        },
        {
          id: 'git-merge',
          label: 'Merge',
          group: 'Git',
          icon: <IconGitMerge className="size-3.5" />,
          keywords: ['merge', 'git', 'branch'],
          action: () => {
            const target = baseBranch?.replace(/^origin\//, '') || 'main';
            return runGitWithFeedback(() => git.merge(target), 'Merge');
          },
        },
      );
    }

    // Workspace — file operations
    if (hasWorktree) {
      items.push({
        id: 'workspace-create-file',
        label: 'Create File',
        group: 'Workspace',
        icon: <IconFilePlus className="size-3.5" />,
        keywords: ['create', 'new', 'file', 'add'],
        enterMode: 'input',
        inputPlaceholder: 'File path relative to workspace root...',
        onInputSubmit: async (path) => {
          const client = getWebSocketClient();
          if (!client || !sessionId) return;
          try {
            const response = await createFile(client, sessionId, path);
            if (response.success) {
              const name = path.split('/').pop() || path;
              useDockviewStore.getState().addFileEditorPanel(path, name);
            }
          } catch (error) {
            console.error('Failed to create file:', error);
          }
        },
      });
    }

    // Panels — always available on session page
    items.push(
      {
        id: 'panel-browser',
        label: 'Add Browser Panel',
        group: 'Panels',
        icon: <IconBrowser className="size-3.5" />,
        keywords: ['browser', 'preview', 'web'],
        action: () => panels.addBrowser(),
      },
      {
        id: 'panel-terminal',
        label: 'Add Terminal Panel',
        group: 'Panels',
        icon: <IconTerminal2 className="size-3.5" />,
        keywords: ['terminal', 'shell', 'console'],
        action: () => panels.addTerminal(),
      },
    );

    if (!isPassthrough) {
      items.push({
        id: 'panel-plan',
        label: 'Add Plan Panel',
        group: 'Panels',
        icon: <IconFileText className="size-3.5" />,
        keywords: ['plan', 'document'],
        action: () => panels.addPlan(),
      });
    }

    items.push({
      id: 'panel-changes',
      label: 'Add Changes Panel',
      group: 'Panels',
      icon: <IconFileDiff className="size-3.5" />,
      keywords: ['changes', 'diff', 'git'],
      action: () => panels.addChanges(),
    });

    return items.map((cmd) => ({ ...cmd, priority: PAGE_PRIORITY }));
  }, [sessionId, git, panels, cancelTurn, baseBranch, isAgentRunning, hasWorktree, isPassthrough, openCommitDialog, openPRDialog, runGitWithFeedback]);

  useRegisterCommands(commands);

  return null;
}
