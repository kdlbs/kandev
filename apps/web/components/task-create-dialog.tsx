'use client';

import { useState, FormEvent } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (title: string, description: string) => void;
  initialValues?: {
    title: string;
    description?: string;
  };
  submitLabel?: string;
}

export function TaskCreateDialog({
  open,
  onOpenChange,
  onSubmit,
  initialValues,
  submitLabel = 'Create',
}: TaskCreateDialogProps) {
  const [title, setTitle] = useState(initialValues?.title ?? '');
  const [description, setDescription] = useState(initialValues?.description ?? '');

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (title.trim()) {
      onSubmit(title.trim(), description.trim());
      setTitle('');
      setDescription('');
      onOpenChange(false);
    }
  };

  const handleCancel = () => {
    setTitle('');
    setDescription('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label>Title</Label>
            <Input
              autoFocus
              required
              placeholder="Enter task title..."
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Description</Label>
            <Textarea
              placeholder="Enter task description (optional)..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" onClick={handleCancel}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit">{submitLabel}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
