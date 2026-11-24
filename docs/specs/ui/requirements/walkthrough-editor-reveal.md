---
status: active
system: ui
created: 2026-11-23
owners:
  - product
---

# Walkthrough Editor Reveal Requirements

## Overview

The walkthrough opens a file so the reader can see the code described by the
active step. The UI owns how the editor presents that range.

## Requirements

### REQ-UI-WALKTHROUGH-EDITOR-REVEAL-001: First-open editor reveal

**Intent:** Start reading the walkthrough at the referenced code as soon as the
editor is ready.

#### Acceptance criteria

- **AC-UI-WALKTHROUGH-EDITOR-REVEAL-001.1:** When a desktop user opens a
  walkthrough whose first step references an offscreen range in a Monaco
  source editor, the editor shall open the file and bring the highlighted range
  into its viewport without a step change. If the range is taller than the
  viewport, the start of the range shall be visible.
- **AC-UI-WALKTHROUGH-EDITOR-REVEAL-001.2:** When the reader leaves a step and
  returns to it, the editor shall show that step's range again. After the
  range is in view, the reader can scroll away while the step remains open.

## Out of scope

- Walkthrough storage, step ordering, and recovery for unavailable files.
- Changes to the walkthrough card, phone sheet, or editor selection settings.
