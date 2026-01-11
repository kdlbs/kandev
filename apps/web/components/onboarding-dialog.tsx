'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { IconArrowRight, IconCheck, IconRocket } from '@tabler/icons-react';
import { setLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS, DEFAULT_BACKEND_URL } from '@/lib/settings/constants';
import { isValidBackendUrl } from '@/lib/websocket/utils';

interface OnboardingDialogProps {
  open: boolean;
  onComplete: () => void;
}

export function OnboardingDialog({ open, onComplete }: OnboardingDialogProps) {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [backendUrl, setBackendUrl] = useState(DEFAULT_BACKEND_URL);
  const [backendUrlError, setBackendUrlError] = useState('');

  const totalSteps = 3;

  const handleBackendUrlChange = (value: string) => {
    setBackendUrl(value);
    setBackendUrlError('');
  };

  const handleBackendUrlNext = () => {
    if (!backendUrl) {
      setBackendUrlError('Backend URL is required');
      return;
    }

    if (!isValidBackendUrl(backendUrl)) {
      setBackendUrlError('Invalid URL format. Must start with http:// or https://');
      return;
    }

    setLocalStorage(STORAGE_KEYS.BACKEND_URL, backendUrl);
    setStep(3);
  };

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
        {step === 1 && (
          <>
            <DialogHeader>
              <div className="flex items-center justify-center mb-4">
                <div className="h-16 w-16 rounded-full bg-primary/10 flex items-center justify-center">
                  <IconRocket className="h-8 w-8 text-primary" />
                </div>
              </div>
              <DialogTitle className="text-center text-2xl">Welcome to KanDev.ai</DialogTitle>
              <DialogDescription className="text-center">
                Your AI-powered development task management system. Let's get you set up in just a
                few steps.
              </DialogDescription>
            </DialogHeader>
            <div className="py-6">
              <div className="space-y-4 text-sm text-muted-foreground">
                <div className="flex items-start gap-3">
                  <div className="h-6 w-6 rounded-full bg-primary/10 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <span className="text-xs font-medium text-primary">1</span>
                  </div>
                  <div>
                    <p className="font-medium text-foreground">Connect to Backend</p>
                    <p className="text-xs">Configure your backend server connection</p>
                  </div>
                </div>
                <div className="flex items-start gap-3">
                  <div className="h-6 w-6 rounded-full bg-primary/10 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <span className="text-xs font-medium text-primary">2</span>
                  </div>
                  <div>
                    <p className="font-medium text-foreground">Create Workspaces</p>
                    <p className="text-xs">Set up your projects and contexts</p>
                  </div>
                </div>
                <div className="flex items-start gap-3">
                  <div className="h-6 w-6 rounded-full bg-primary/10 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <span className="text-xs font-medium text-primary">3</span>
                  </div>
                  <div>
                    <p className="font-medium text-foreground">Configure Agents</p>
                    <p className="text-xs">Set up your AI agent profiles</p>
                  </div>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={handleSkip}>
                Skip for now
              </Button>
              <Button onClick={() => setStep(2)}>
                Get Started
                <IconArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </DialogFooter>
          </>
        )}

        {step === 2 && (
          <>
            <DialogHeader>
              <div className="flex items-center justify-between mb-2">
                <div className="text-xs text-muted-foreground">Step 1 of {totalSteps - 1}</div>
                <div className="flex gap-1">
                  <div className="h-1.5 w-8 rounded-full bg-primary" />
                  <div className="h-1.5 w-8 rounded-full bg-muted" />
                </div>
              </div>
              <DialogTitle>Backend Server Connection</DialogTitle>
              <DialogDescription>
                Enter the URL of your KanDev backend server. This is required for real-time task
                synchronization and agent communication.
              </DialogDescription>
            </DialogHeader>
            <div className="py-4 space-y-4">
              <div className="space-y-2">
                <Label htmlFor="backend-url">Backend Server URL</Label>
                <Input
                  id="backend-url"
                  type="url"
                  value={backendUrl}
                  onChange={(e) => handleBackendUrlChange(e.target.value)}
                  placeholder="http://localhost:8080"
                  className={backendUrlError ? 'border-red-500' : ''}
                  autoFocus
                />
                {backendUrlError && <p className="text-xs text-red-500">{backendUrlError}</p>}
                <p className="text-xs text-muted-foreground">
                  Default: http://localhost:8080 for local development
                </p>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={handleSkip}>
                Skip for now
              </Button>
              <Button onClick={handleBackendUrlNext}>
                Continue
                <IconArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </DialogFooter>
          </>
        )}

        {step === 3 && (
          <>
            <DialogHeader>
              <div className="flex items-center justify-between mb-2">
                <div className="text-xs text-muted-foreground">Step 2 of {totalSteps - 1}</div>
                <div className="flex gap-1">
                  <div className="h-1.5 w-8 rounded-full bg-primary" />
                  <div className="h-1.5 w-8 rounded-full bg-primary" />
                </div>
              </div>
              <DialogTitle>Next Steps</DialogTitle>
              <DialogDescription>
                You're almost ready! Complete these final configuration steps in Settings.
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <div className="space-y-4">
                <div className="rounded-lg border p-4 space-y-3">
                  <div className="flex items-start gap-3">
                    <div className="h-5 w-5 rounded-full border-2 flex items-center justify-center flex-shrink-0 mt-0.5">
                      <div className="h-2 w-2 rounded-full bg-muted" />
                    </div>
                    <div>
                      <p className="font-medium text-sm">Create a Workspace</p>
                      <p className="text-xs text-muted-foreground">
                        Set up your first workspace and add project repositories
                      </p>
                    </div>
                  </div>
                  <div className="flex items-start gap-3">
                    <div className="h-5 w-5 rounded-full border-2 flex items-center justify-center flex-shrink-0 mt-0.5">
                      <div className="h-2 w-2 rounded-full bg-muted" />
                    </div>
                    <div>
                      <p className="font-medium text-sm">Configure Projects</p>
                      <p className="text-xs text-muted-foreground">
                        Add project contexts and custom task columns
                      </p>
                    </div>
                  </div>
                  <div className="flex items-start gap-3">
                    <div className="h-5 w-5 rounded-full border-2 flex items-center justify-center flex-shrink-0 mt-0.5">
                      <div className="h-2 w-2 rounded-full bg-muted" />
                    </div>
                    <div>
                      <p className="font-medium text-sm">Set Up Agent Profiles</p>
                      <p className="text-xs text-muted-foreground">
                        Configure your AI agents with preferred models and settings
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={handleSkip}>
                Skip for now
              </Button>
              <Button onClick={handleGoToSettings}>
                <IconCheck className="mr-2 h-4 w-4" />
                Go to Settings
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
