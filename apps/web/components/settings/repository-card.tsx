'use client';

import { useState } from 'react';
import { IconGitBranch, IconTrash, IconEdit, IconCheck, IconX, IconPlus } from '@tabler/icons-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import type { Repository } from '@/lib/settings/types';

type RepositoryCardProps = {
  repository: Repository;
  onUpdate: (repo: Repository) => void;
  onDelete: () => void;
};

export function RepositoryCard({ repository, onUpdate, onDelete }: RepositoryCardProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const [editedRepo, setEditedRepo] = useState<Repository>(repository);

  const handleSave = () => {
    onUpdate(editedRepo);
    setIsEditing(false);
  };

  const handleCancel = () => {
    setEditedRepo(repository);
    setIsEditing(false);
  };

  const handleFolderPick = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.webkitdirectory = true;
    input.onchange = (event: Event) => {
      const target = event.target as HTMLInputElement | null;
      const files = target?.files;
      if (files && files[0]) {
        const path = files[0].webkitRelativePath.split('/')[0];
        setEditedRepo({ ...editedRepo, path: `/${path}` });
      }
    };
    input.click();
  };

  const handleAddCustomScript = () => {
    setEditedRepo({
      ...editedRepo,
      customScripts: [
        ...editedRepo.customScripts,
        { id: crypto.randomUUID(), name: '', command: '#!/bin/bash\n' },
      ],
    });
  };

  const handleRemoveCustomScript = (id: string) => {
    setEditedRepo({
      ...editedRepo,
      customScripts: editedRepo.customScripts.filter((s) => s.id !== id),
    });
  };

  const handleUpdateCustomScript = (id: string, field: 'name' | 'command', value: string) => {
    setEditedRepo({
      ...editedRepo,
      customScripts: editedRepo.customScripts.map((s) =>
        s.id === id ? { ...s, [field]: value } : s
      ),
    });
  };

  if (isEditing) {
    return (
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Repository Name</Label>
              <Input
                value={editedRepo.name}
                onChange={(e) => setEditedRepo({ ...editedRepo, name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label>Directory Path</Label>
              <div className="flex gap-2">
                <Input
                  value={editedRepo.path}
                  onChange={(e) => setEditedRepo({ ...editedRepo, path: e.target.value })}
                  placeholder="/path/to/repository"
                />
                <Button type="button" variant="outline" onClick={handleFolderPick}>
                  Browse
                </Button>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Setup Script</Label>
              <Textarea
                value={editedRepo.setupScript}
                onChange={(e) => setEditedRepo({ ...editedRepo, setupScript: e.target.value })}
                placeholder="#!/bin/bash&#10;npm install"
                rows={3}
                className="font-mono text-sm"
              />
            </div>

            <div className="space-y-2">
              <Label>Cleanup Script</Label>
              <Textarea
                value={editedRepo.cleanupScript}
                onChange={(e) => setEditedRepo({ ...editedRepo, cleanupScript: e.target.value })}
                placeholder="#!/bin/bash&#10;rm -rf node_modules"
                rows={3}
                className="font-mono text-sm"
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Custom Scripts</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleAddCustomScript}
                >
                  <IconPlus className="h-4 w-4 mr-1" />
                  Add Script
                </Button>
              </div>
              <div className="space-y-3">
                {editedRepo.customScripts.map((script) => (
                  <div key={script.id} className="border rounded-md p-3 space-y-2">
                    <div className="flex gap-2">
                      <Input
                        placeholder="Script name"
                        value={script.name}
                        onChange={(e) =>
                          handleUpdateCustomScript(script.id, 'name', e.target.value)
                        }
                        className="flex-1"
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        onClick={() => handleRemoveCustomScript(script.id)}
                      >
                        <IconX className="h-4 w-4" />
                      </Button>
                    </div>
                    <Textarea
                      placeholder="#!/bin/bash&#10;npm run dev"
                      value={script.command}
                      onChange={(e) =>
                        handleUpdateCustomScript(script.id, 'command', e.target.value)
                      }
                      rows={3}
                      className="font-mono text-sm"
                    />
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
        <div className="space-y-3">
          <div className="flex items-start justify-between">
            <div className="flex items-start gap-3 flex-1">
              <div className="p-2 bg-muted rounded-md">
                <IconGitBranch className="h-4 w-4" />
              </div>
              <div className="flex-1">
                <h4 className="font-medium">{repository.name}</h4>
                <p className="text-sm text-muted-foreground">{repository.path}</p>
                {repository.customScripts.length > 0 && (
                  <div className="flex gap-1 mt-2 flex-wrap">
                    {repository.customScripts.map((script) => (
                      <Badge key={script.id} variant="secondary" className="text-xs">
                        {script.name}
                      </Badge>
                    ))}
                  </div>
                )}
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

          {isExpanded && (
            <div className="pl-11 space-y-2 text-sm">
              <div>
                <p className="font-medium">Setup:</p>
                <pre className="bg-muted p-2 rounded text-xs overflow-x-auto">
                  {repository.setupScript}
                </pre>
              </div>
              <div>
                <p className="font-medium">Cleanup:</p>
                <pre className="bg-muted p-2 rounded text-xs overflow-x-auto">
                  {repository.cleanupScript}
                </pre>
              </div>
            </div>
          )}

          {(repository.setupScript || repository.cleanupScript) && (
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              className="pl-11 text-xs text-muted-foreground hover:text-foreground"
            >
              {isExpanded ? 'Hide' : 'Show'} scripts
            </button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
