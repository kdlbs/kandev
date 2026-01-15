'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import type { Task } from '@/lib/types/http';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useRepositories } from '@/hooks/use-repositories';
import { useTaskAgent } from '@/hooks/use-task-agent';
import { useTaskComments } from '@/hooks/use-task-comments';

type TaskPageClientProps = {
  task: Task | null;
};

export default function TaskPage({ task: initialTask }: TaskPageClientProps) {
  const [isMounted, setIsMounted] = useState(false);
  const task = initialTask;

  // Custom hooks for state management
  const { isAgentRunning, isAgentLoading, worktreePath, worktreeBranch, handleStartAgent, handleStopAgent } = useTaskAgent(task);
  const { isLoading: isLoadingComments } = useTaskComments(task?.id ?? null);

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);

  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const repository = useMemo(
    () => repositories.find((item) => item.local_path === task?.repository_url) ?? null,
    [repositories, task?.repository_url]
  );

  const handleSendMessage = useCallback(async (content: string) => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    try {
      await client.request('comment.add', { task_id: task.id, content }, 10000);
    } catch (error) {
      console.error('Failed to send comment:', error);
    }
  }, [task]);

  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
        <TaskTopBar
          taskTitle={task?.title}
          taskDescription={task?.description}
          baseBranch={task?.branch ?? undefined}
          onStartAgent={handleStartAgent}
          onStopAgent={handleStopAgent}
          isAgentRunning={isAgentRunning}
          isAgentLoading={isAgentLoading}
          worktreePath={worktreePath}
          worktreeBranch={worktreeBranch}
          repositoryPath={task?.repository_url ?? null}
          repositoryName={repository?.name ?? null}
        />

        <TaskLayout
          taskId={task?.id ?? null}
          taskDescription={task?.description}
          isLoadingComments={isLoadingComments}
          isAgentWorking={isAgentRunning}
          onSendMessage={handleSendMessage}
        />
      </div>
    </TooltipProvider>
  );
}
