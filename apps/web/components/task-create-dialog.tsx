'use client';

import { useEffect, useState, FormEvent } from 'react';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogFooter,
  AlertDialogCancel,
} from '@/components/ui/alert-dialog';
import { Field, FieldLabel } from '@/components/ui/field';
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
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (!open) return;
    setTitle(initialValues?.title ?? '');
    setDescription(initialValues?.description ?? '');
  }, [open, initialValues]);

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
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        size="default"
        overlayProps={{
          onClick: () => onOpenChange(false),
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}</AlertDialogTitle>
        </AlertDialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel>Title</FieldLabel>
            <Input
              autoFocus
              required
              placeholder="Enter task title..."
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>Description</FieldLabel>
            <Textarea
              placeholder="Enter task description (optional)..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </Field>
          <AlertDialogFooter>
            <AlertDialogCancel type="button" onClick={handleCancel}>
              Cancel
            </AlertDialogCancel>
            <Button type="submit">{submitLabel}</Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  );
}
