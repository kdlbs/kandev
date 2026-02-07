'use client';

import { useCallback, useState } from 'react';
import { IconX, IconMessageQuestion } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { getBackendConfig } from '@/lib/config';
import type { Message, ClarificationRequestMetadata, ClarificationAnswer } from '@/lib/types/http';

type ClarificationInputOverlayProps = {
  message: Message;
  onResolved: () => void;
};

/**
 * Inline clarification UI - simple numbered text options like Conductor.
 */
export function ClarificationInputOverlay({ message, onResolved }: ClarificationInputOverlayProps) {
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  const [customText, setCustomText] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmitOption = useCallback(async (optionId: string) => {
    if (!metadata?.pending_id || isSubmitting) return;
    setIsSubmitting(true);
    try {
      const answer: ClarificationAnswer = {
        question_id: metadata.question.id,
        selected_options: [optionId],
      };

      const { apiBaseUrl } = getBackendConfig();
      const response = await fetch(`${apiBaseUrl}/api/v1/clarification/${metadata.pending_id}/respond`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ answers: [answer], rejected: false }),
      });
      if (response.ok) {
        onResolved();
      } else if (response.status === 410) {
        // Clarification expired (agent timed out)
        // Backend has marked the message as expired, so reload to clear the UI
        const data = await response.json().catch(() => ({}));
        console.warn('Clarification expired:', data);
        // Force reload to fetch updated messages (clarification no longer pending)
        window.location.reload();
      } else {
        console.error('Failed to submit clarification response:', response.status, response.statusText);
      }
    } catch (error) {
      console.error('Failed to submit clarification response:', error);
    } finally {
      setIsSubmitting(false);
    }
  }, [metadata, isSubmitting, onResolved]);

  const handleSubmitCustom = useCallback(async () => {
    if (!metadata?.pending_id || isSubmitting || !customText.trim()) return;
    setIsSubmitting(true);
    try {
      const answer: ClarificationAnswer = {
        question_id: metadata.question.id,
        selected_options: [],
        custom_text: customText.trim(),
      };

      const { apiBaseUrl } = getBackendConfig();
      const response = await fetch(`${apiBaseUrl}/api/v1/clarification/${metadata.pending_id}/respond`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ answers: [answer], rejected: false }),
      });
      if (response.ok) {
        onResolved();
      } else if (response.status === 410) {
        // Clarification expired (agent timed out)
        // Backend has marked the message as expired, so reload to clear the UI
        const data = await response.json().catch(() => ({}));
        console.warn('Clarification expired:', data);
        // Force reload to fetch updated messages (clarification no longer pending)
        window.location.reload();
      } else {
        console.error('Failed to submit clarification response:', response.status, response.statusText);
      }
    } catch (error) {
      console.error('Failed to submit clarification response:', error);
    } finally {
      setIsSubmitting(false);
    }
  }, [metadata, isSubmitting, customText, onResolved]);

  const handleSkip = useCallback(async () => {
    if (!metadata?.pending_id || isSubmitting) return;
    setIsSubmitting(true);
    try {
      const { apiBaseUrl } = getBackendConfig();
      const response = await fetch(`${apiBaseUrl}/api/v1/clarification/${metadata.pending_id}/respond`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ answers: [], rejected: true, reject_reason: 'User skipped' }),
      });
      if (response.ok) {
        onResolved();
      } else if (response.status === 410) {
        // Clarification expired (agent timed out)
        const data = await response.json().catch(() => ({}));
        console.warn('Clarification expired:', data);
        alert('This question has timed out. The agent has already moved on. Please refresh the page.');
        onResolved(); // Clear the UI even though it failed
      } else {
        console.error('Failed to skip clarification:', response.status, response.statusText);
      }
    } catch (error) {
      console.error('Failed to skip clarification:', error);
    } finally {
      setIsSubmitting(false);
    }
  }, [metadata, isSubmitting, onResolved]);

  if (!metadata?.question) {
    return null;
  }

  const question = metadata.question;

  return (
    <div className="relative px-3 py-2">
      {/* Close/skip button */}
      <button
        type="button"
        onClick={handleSkip}
        disabled={isSubmitting}
        className="absolute top-2 right-3 text-muted-foreground hover:text-foreground z-10"
      >
        <IconX className="h-4 w-4" />
      </button>

      {/* Content */}
      <div className="pr-6">
        {/* Question text */}
        <div className="flex items-start gap-2 mb-1">
          <IconMessageQuestion className="h-4 w-4 text-blue-500 flex-shrink-0 mt-0.5" />
          <p className="text-sm text-foreground">{question.prompt}</p>
        </div>

        {/* Options with bullets - indented to align with question text */}
        <div className="space-y-0.5 mb-1.5 ml-6">
          {question.options.map((option) => (
            <button
              key={option.option_id}
              type="button"
              onClick={() => handleSubmitOption(option.option_id)}
              disabled={isSubmitting}
              className={cn(
                'flex items-start gap-2 w-full text-left text-xs rounded px-1.5 py-0.5 -ml-1.5 transition-colors',
                'hover:bg-blue-500/15 hover:text-blue-600 dark:hover:text-blue-400',
                isSubmitting ? 'opacity-50 cursor-not-allowed' : 'text-foreground/80'
              )}
            >
              <span className="text-muted-foreground flex-shrink-0">•</span>
              <span>{option.label}</span>
              {option.description && (
                <span className="text-muted-foreground/60">— {option.description}</span>
              )}
            </button>
          ))}
        </div>

        {/* Custom input row - indented to align with options */}
        <div className="flex items-center gap-2 ml-6">
          <span className="text-muted-foreground flex-shrink-0">•</span>
          <input
            type="text"
            placeholder="Type something..."
            value={customText}
            onChange={(e) => setCustomText(e.target.value)}
            disabled={isSubmitting}
            className="flex-1 text-sm bg-transparent placeholder:text-muted-foreground focus:outline-none"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey && customText.trim()) {
                e.preventDefault();
                handleSubmitCustom();
              }
            }}
          />
        </div>
      </div>
    </div>
  );
}

