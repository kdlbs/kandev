---
name: interview-me
description: Clarify what the user actually wants before specs, plans, or code. Use when an ask is underspecified, when the user says "interview me", "grill me", "stress-test my thinking", or when you would otherwise fill in important product/architecture assumptions silently.
---

# Interview Me

Use this before `/spec-driven-development`, `/spec`, or `/plan` when the requested outcome is not clear enough to implement without guessing.

## When To Use

Use when the ask is missing one or more of:
- Who the user/operator is
- Why this matters now
- What success looks like
- The binding constraint or tradeoff
- Explicit out-of-scope boundaries

Skip for mechanical edits, obvious bug fixes, pure information requests, or when the user explicitly asks for speed over clarification.

## Process

### 1. State A Hypothesis

Write one sentence plus a confidence number.

```text
HYPOTHESIS: You want workspace switching to preserve task context because agents lose momentum when users navigate between workspaces.
CONFIDENCE: 45% - missing: who feels the pain, what "preserve" means, and what counts as done.
```

### 2. Ask Focused Questions

Format every question with a guess.

```text
Q: Is this primarily for human users switching between workspaces, or for office agents operating across workspaces?
GUESS: Human users, because the pain sounds navigation-related rather than automation-related.
```

If a tool is available that supports multiple questions in one turn, such as `ask_user_question_kandev`, ask 2-4 focused questions together. Keep each question short, include your guess in the prompt or options, and make the options concrete enough that the user can answer quickly.

If no multi-question tool is available, ask one question at a time in chat. Do not send a long questionnaire.

### 3. Probe Convention-Sounding Answers

If the user says "modern", "scalable", "best practice", "dashboard", "robust", or "clean architecture" without concrete outcomes, ask:

```text
If you did not have to justify this as best practice, what would you actually want?
```

### 4. Restate Intent

When confidence is high, restate in this shape:

```text
Here's what I think you want:
- Outcome:
- User:
- Why now:
- Success:
- Constraint:
- Out of scope:

Yes / no / refine?
```

Do not proceed to `/spec`, `/plan`, or implementation until the user explicitly confirms or corrects the restate.

## Output

The deliverable is a confirmed statement of intent. If the user wants it persisted, save it only after confirmation, usually in a feature spec via `/spec`; avoid creating standalone intent docs unless explicitly requested.

## Red Flags

- Long open-ended questionnaires with no guesses
- A question with no guess attached
- Accepting "whatever you think" as confirmation
- Starting a spec or plan before the user confirms the restate
- No explicit out-of-scope line
