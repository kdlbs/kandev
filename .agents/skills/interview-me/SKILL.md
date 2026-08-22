---
name: interview-me
description: Clarify what the user wants before requirements, system design, plans, or code. Use when an ask is underspecified, when the user asks for an interview or stress test, or when important product or architecture assumptions are missing.
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

If the active harness provides a native user-question UI that supports multiple
questions in one turn, ask 2-4 focused questions together. Keep each question
short, include your guess in the prompt or options, and make the options
concrete enough that the user can answer quickly.

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

The deliverable is a confirmed statement of intent. If the user wants it
persisted, save it only after confirmation. Use `/spec` to update the owning
system requirement. Do not create a standalone intent document unless the user
explicitly requests one.

## Red Flags

- Long open-ended questionnaires with no guesses
- A question with no guess attached
- Accepting "whatever you think" as confirmation
- Starting a spec or plan before the user confirms the restate
- No explicit out-of-scope line
