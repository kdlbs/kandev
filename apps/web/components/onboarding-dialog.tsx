'use client';

import { useRouter } from 'next/navigation';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@kandev/ui/dialog';
import { Button } from '@kandev/ui/button';
import { IconCheck, IconRocket } from '@tabler/icons-react';
interface OnboardingDialogProps {
  open: boolean;
  onComplete: () => void;
}

export function OnboardingDialog({ open, onComplete }: OnboardingDialogProps) {
  const router = useRouter();

  const handleSkip = () => {
    onComplete();
  };

  const handleGoToSettings = () => {
    onComplete();
    router.push('/settings');
  };

  return (
    <Dialog open={open} onOpenChange={() => {}}>
      <DialogContent className="sm:max-w-[500px]" showCloseButton={false}>
        <>
          <DialogHeader>
            <div className="flex items-center justify-center mb-4">
              <div className="h-16 w-16 rounded-full bg-primary/10 flex items-center justify-center">
                <IconRocket className="h-8 w-8 text-primary" />
              </div>
            </div>
            <DialogTitle className="text-center text-2xl">Welcome to KanDev.ai</DialogTitle>
            <DialogDescription className="text-center">
              Your AI-powered development task management system. You are ready to start building.
            </DialogDescription>
          </DialogHeader>
          <div className="py-6">
            <div className="space-y-4 text-sm text-muted-foreground">
              <div className="rounded-lg border p-4 space-y-3">
                <div className="flex items-start gap-3">
                  <div className="h-5 w-5 rounded-full border-2 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <div className="h-2 w-2 rounded-full bg-muted" />
                  </div>
                  <div>
                    <p className="font-medium text-sm">Agents & Environments</p>
                    <p className="text-xs text-muted-foreground">
                      Configure agent profiles, environments, and credentials in Settings.
                    </p>
                  </div>
                </div>
                <div className="flex items-start gap-3">
                  <div className="h-5 w-5 rounded-full border-2 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <div className="h-2 w-2 rounded-full bg-muted" />
                  </div>
                  <div>
                    <p className="font-medium text-sm">Board Columns</p>
                    <p className="text-xs text-muted-foreground">
                      Customize your columns anytime to match your workflow.
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={handleSkip}>
              Start using KanDev
            </Button>
            <Button onClick={handleGoToSettings}>
              <IconCheck className="mr-2 h-4 w-4" />
              Go to Settings
            </Button>
          </DialogFooter>
        </>
      </DialogContent>
    </Dialog>
  );
}
