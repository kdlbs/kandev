'use client';

import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { Textarea } from '@kandev/ui/textarea';
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard';

type WorkflowExportDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  json: string;
};

export function WorkflowExportDialog({ open, onOpenChange, title, json }: WorkflowExportDialogProps) {
  const { copied, copy } = useCopyToClipboard();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <Textarea
          readOnly
          value={json}
          className="font-mono text-xs max-h-96 overflow-y-auto"
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Close
          </Button>
          <Button onClick={() => copy(json)} className="cursor-pointer">
            {copied ? <IconCheck className="h-4 w-4 mr-2" /> : <IconCopy className="h-4 w-4 mr-2" />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
