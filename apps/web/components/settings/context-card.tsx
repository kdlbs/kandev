'use client';

import { useState } from 'react';
import { IconFolder, IconTrash, IconEdit, IconCheck, IconX, IconPlus } from '@tabler/icons-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import type { Context, Column } from '@/lib/settings/types';

type ContextCardProps = {
  context: Context;
  onUpdate: (context: Context) => void;
  onDelete: () => void;
};

const COLOR_OPTIONS = [
  { value: 'bg-neutral-400', label: 'Gray' },
  { value: 'bg-red-500', label: 'Red' },
  { value: 'bg-orange-500', label: 'Orange' },
  { value: 'bg-yellow-500', label: 'Yellow' },
  { value: 'bg-green-500', label: 'Green' },
  { value: 'bg-blue-500', label: 'Blue' },
  { value: 'bg-indigo-500', label: 'Indigo' },
  { value: 'bg-violet-500', label: 'Violet' },
  { value: 'bg-cyan-500', label: 'Cyan' },
  { value: 'bg-sky-500', label: 'Sky' },
];

export function ContextCard({ context, onUpdate, onDelete }: ContextCardProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [editedContext, setEditedContext] = useState<Context>(context);

  const handleSave = () => {
    onUpdate(editedContext);
    setIsEditing(false);
  };

  const handleCancel = () => {
    setEditedContext(context);
    setIsEditing(false);
  };

  const handleAddColumn = () => {
    setEditedContext({
      ...editedContext,
      columns: [
        ...editedContext.columns,
        { id: crypto.randomUUID(), title: '', color: 'bg-neutral-400' },
      ],
    });
  };

  const handleRemoveColumn = (id: string) => {
    setEditedContext({
      ...editedContext,
      columns: editedContext.columns.filter((c) => c.id !== id),
    });
  };

  const handleUpdateColumn = (id: string, field: 'title' | 'color' | 'id', value: string) => {
    setEditedContext({
      ...editedContext,
      columns: editedContext.columns.map((c) =>
        c.id === id ? { ...c, [field]: value } : c
      ),
    });
  };

  if (isEditing) {
    return (
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Context Name</Label>
              <Input
                value={editedContext.name}
                onChange={(e) => setEditedContext({ ...editedContext, name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Kanban Columns</Label>
                <Button type="button" variant="outline" size="sm" onClick={handleAddColumn}>
                  <IconPlus className="h-4 w-4 mr-1" />
                  Add Column
                </Button>
              </div>
              <div className="space-y-2">
                {editedContext.columns.map((column, index) => (
                  <div key={column.id} className="flex gap-2 items-start">
                    <div className="flex-1 grid grid-cols-2 gap-2">
                      <div className="space-y-1">
                        <Input
                          placeholder="Column title"
                          value={column.title}
                          onChange={(e) =>
                            handleUpdateColumn(column.id, 'title', e.target.value)
                          }
                        />
                      </div>
                      <div className="space-y-1">
                        <select
                          value={column.color}
                          onChange={(e) =>
                            handleUpdateColumn(column.id, 'color', e.target.value)
                          }
                          className="w-full h-9 px-3 rounded-md border border-input bg-background text-sm"
                        >
                          {COLOR_OPTIONS.map((option) => (
                            <option key={option.value} value={option.value}>
                              {option.label}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="col-span-2 space-y-1">
                        <Input
                          placeholder="Column ID (e.g., todo, in-progress)"
                          value={column.id}
                          onChange={(e) =>
                            handleUpdateColumn(column.id, 'id', e.target.value)
                          }
                        />
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={() => handleRemoveColumn(column.id)}
                    >
                      <IconX className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>

            <div className="flex gap-2 justify-end">
              <Button variant="outline" onClick={handleCancel}>
                <IconX className="h-4 w-4 mr-2" />
                Cancel
              </Button>
              <Button onClick={handleSave}>
                <IconCheck className="h-4 w-4 mr-2" />
                Save
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="hover:bg-accent transition-colors cursor-pointer" onClick={() => setIsEditing(true)}>
      <CardContent className="py-4">
        <div className="flex items-start justify-between">
          <div className="flex items-start gap-3 flex-1">
            <div className="p-2 bg-muted rounded-md">
              <IconFolder className="h-4 w-4" />
            </div>
            <div className="flex-1">
              <h4 className="font-medium">{context.name}</h4>
              <div className="flex flex-wrap gap-1 mt-2">
                {context.columns.map((column) => {
                  // Map Tailwind bg colors to actual hex values
                  const colorMap: Record<string, string> = {
                    'bg-neutral-400': '#a3a3a3',
                    'bg-red-500': '#ef4444',
                    'bg-orange-500': '#f97316',
                    'bg-yellow-500': '#eab308',
                    'bg-green-500': '#22c55e',
                    'bg-blue-500': '#3b82f6',
                    'bg-indigo-500': '#6366f1',
                    'bg-violet-500': '#8b5cf6',
                    'bg-cyan-500': '#06b6d4',
                    'bg-sky-500': '#0ea5e9',
                  };

                  return (
                    <Badge
                      key={column.id}
                      variant="outline"
                      className="text-xs"
                      style={{
                        borderLeftWidth: '3px',
                        borderLeftColor: colorMap[column.color] || '#a3a3a3',
                      }}
                    >
                      {column.title}
                    </Badge>
                  );
                })}
              </div>
            </div>
          </div>
          <div className="flex gap-1" onClick={(e) => e.stopPropagation()}>
            <Button variant="ghost" size="icon" onClick={() => setIsEditing(true)}>
              <IconEdit className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={onDelete}>
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
