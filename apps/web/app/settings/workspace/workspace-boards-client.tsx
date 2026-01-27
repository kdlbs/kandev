'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconLayoutColumns, IconPlus, IconRobot, IconCheck, IconArrowRight } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { RadioGroup, RadioGroupItem } from '@kandev/ui/radio-group';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@kandev/ui/dialog';
import { SettingsSection } from '@/components/settings/settings-section';
import { BoardCard } from '@/components/settings/board-card';
import { generateUUID } from '@/lib/utils';
import { cn } from '@kandev/ui/lib/utils';
import {
  createBoardAction,
  deleteBoardAction,
  updateBoardAction,
} from '@/app/actions/workspaces';
import type { Board, Workspace, WorkflowTemplate } from '@/lib/types/http';

type WorkspaceBoardsClientProps = {
  workspace: Workspace | null;
  boards: Board[];
  workflowTemplates: WorkflowTemplate[];
};

export function WorkspaceBoardsClient({
  workspace,
  boards,
  workflowTemplates,
}: WorkspaceBoardsClientProps) {
  const router = useRouter();
  const [boardItems, setBoardItems] = useState<Board[]>(boards);
  const [savedBoardItems, setSavedBoardItems] = useState<Board[]>(boards);

  // Dialog state for creating a new board
  const [isAddBoardDialogOpen, setIsAddBoardDialogOpen] = useState(false);
  const [newBoardName, setNewBoardName] = useState('');
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);

  // Get selected template steps for preview
  const selectedTemplate = workflowTemplates.find((t) => t.id === selectedTemplateId);
  const previewSteps = selectedTemplate?.default_steps ?? [];

  const savedBoardsById = useMemo(() => {
    return new Map(savedBoardItems.map((board) => [board.id, board]));
  }, [savedBoardItems]);

  const isBoardDirty = (board: Board) => {
    const saved = savedBoardsById.get(board.id);
    if (!saved) return true;
    if (board.name !== saved.name || board.description !== saved.description) return true;
    return false;
  };

  const handleOpenAddBoardDialog = () => {
    setNewBoardName('');
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddBoardDialogOpen(true);
  };

  const handleCreateBoard = () => {
    if (!workspace) return;

    const draftBoard: Board = {
      id: `temp-${generateUUID()}`,
      workspace_id: workspace.id,
      name: newBoardName.trim() || 'New Board',
      description: '',
      workflow_template_id: selectedTemplateId,
      created_at: '',
      updated_at: '',
    };

    setBoardItems((prev) => [draftBoard, ...prev]);
    setIsAddBoardDialogOpen(false);
  };

  const handleUpdateBoard = (boardId: string, updates: { name?: string; description?: string }) => {
    setBoardItems((prev) =>
      prev.map((board) => (board.id === boardId ? { ...board, ...updates } : board))
    );
  };

  const handleDeleteBoard = async (boardId: string) => {
    if (boardId.startsWith('temp-')) {
      setBoardItems((prev) => prev.filter((board) => board.id !== boardId));
      return;
    }
    await deleteBoardAction(boardId);
    setBoardItems((prev) => prev.filter((board) => board.id !== boardId));
    setSavedBoardItems((prev) => prev.filter((board) => board.id !== boardId));
  };

  const handleSaveBoard = async (boardId: string) => {
    const board = boardItems.find((item) => item.id === boardId);
    if (!board) return;
    if (boardId.startsWith('temp-')) {
      const name = board.name.trim() || 'New Board';
      const createdBoard = await createBoardAction({
        workspace_id: workspace?.id ?? board.workspace_id,
        name,
        workflow_template_id: board.workflow_template_id ?? undefined,
      });
      // Backend creates workflow steps automatically from the template
      setBoardItems((prev) => prev.map((item) => (item.id === boardId ? createdBoard : item)));
      setSavedBoardItems((prev) => [{ ...createdBoard }, ...prev]);
      // Refresh the page to load the workflow steps created by the backend
      router.refresh();
      return;
    }
    // For existing boards, only update board name/description
    const updates: { name?: string; description?: string } = {};
    if (board.name.trim()) {
      updates.name = board.name.trim();
    }
    if (Object.keys(updates).length) {
      await updateBoardAction(boardId, updates);
    }
    setBoardItems((prev) =>
      prev.map((item) => (item.id === boardId ? { ...item, ...updates } : item))
    );
    setSavedBoardItems((prev) =>
      prev.some((item) => item.id === boardId)
        ? prev.map((item) =>
            item.id === boardId ? { ...board, ...updates } : item
          )
        : [...prev, { ...board, ...updates }]
    );
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
            Manage boards for this workspace.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconLayoutColumns className="h-5 w-5" />}
        title="Boards"
        description="Boards in this workspace"
        action={
          <Button size="sm" onClick={handleOpenAddBoardDialog}>
            <IconPlus className="h-4 w-4 mr-2" />
            Add Board
          </Button>
        }
      >
        <div className="grid gap-3">
          {boardItems.map((board) => (
            <BoardCard
              key={board.id}
              board={board}
              isBoardDirty={isBoardDirty(board)}
              onUpdateBoard={(updates) => handleUpdateBoard(board.id, updates)}
              onDeleteBoard={() => handleDeleteBoard(board.id)}
              onSaveBoard={() => handleSaveBoard(board.id)}
            />
          ))}
        </div>
      </SettingsSection>

      {/* Create Board Dialog */}
      <Dialog open={isAddBoardDialogOpen} onOpenChange={setIsAddBoardDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Create Board</DialogTitle>
          </DialogHeader>

          <div className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="boardName">Board Name</Label>
              <Input
                id="boardName"
                placeholder="My Project Board"
                value={newBoardName}
                onChange={(e) => setNewBoardName(e.target.value)}
              />
            </div>

            {workflowTemplates.length > 0 && (
              <div className="space-y-2">
                <Label>Workflow Template</Label>
                <RadioGroup
                  value={selectedTemplateId ?? 'custom'}
                  onValueChange={(value) =>
                    setSelectedTemplateId(value === 'custom' ? null : value)
                  }
                >
                  {workflowTemplates.map((template) => (
                    <div key={template.id} className="flex items-start space-x-3">
                      <RadioGroupItem value={template.id} id={template.id} className="mt-1" />
                      <label htmlFor={template.id} className="flex flex-col cursor-pointer">
                        <span className="font-medium">{template.name}</span>
                        {template.description && (
                          <span className="text-sm text-muted-foreground">
                            {template.description}
                          </span>
                        )}
                      </label>
                    </div>
                  ))}
                  <div className="flex items-start space-x-3">
                    <RadioGroupItem value="custom" id="custom" className="mt-1" />
                    <label htmlFor="custom" className="flex flex-col cursor-pointer">
                      <span className="font-medium">Custom</span>
                      <span className="text-sm text-muted-foreground">
                        Start with basic columns (Todo, In Progress, Review, Done)
                      </span>
                    </label>
                  </div>
                </RadioGroup>
              </div>
            )}

            {/* Workflow Preview */}
            {previewSteps.length > 0 && (
              <div className="border rounded-lg p-4 bg-muted/50">
                <Label className="text-sm">Workflow Preview</Label>
                <div className="flex items-center gap-2 mt-2 overflow-x-auto pb-2">
                  {previewSteps.map((step, i) => (
                    <div key={`step-${i}`} className="flex items-center gap-2">
                      <div
                        className={cn(
                          'px-3 py-1.5 rounded text-sm whitespace-nowrap flex items-center gap-1',
                          step.color || 'bg-slate-500',
                          'text-white'
                        )}
                      >
                        {step.name}
                        {step.behaviors?.autoStartAgent && (
                          <IconRobot className="h-3 w-3" />
                        )}
                        {step.behaviors?.requireApproval && (
                          <IconCheck className="h-3 w-3" />
                        )}
                      </div>
                      {i < previewSteps.length - 1 && (
                        <IconArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsAddBoardDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreateBoard}>Create Board</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
