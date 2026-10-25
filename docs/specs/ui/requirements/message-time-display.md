---
status: draft
system: ui
created: 2026-10-03
owners:
  - kandev
---

# Message Time Display Requirements

## Overview

Every message in a session transcript shows its creation time in the message footer. Today the footer always renders a relative label ("5m ago") with the full date and time as a tooltip. A user who reviews a long session after the fact needs the exact moment on the line itself, and a user who watches a live session prefers the relative form. This capability lets each user choose the transcript timestamp form and keeps the form they did not choose reachable through the tooltip.

The UI system owns this contract because it is a reusable presentation preference for transcript chrome, alongside [transcript navigation settings](transcript-navigation-settings.md). Message content, ordering, and persistence stay with the task and agent systems.

## Terminology

- **Message time label:** The visible creation-time text in a transcript message footer.
- **Relative form:** A phrase for the message's age in the interface language at any age, such as "5m ago", "yesterday", or "3 weeks ago". It is never a calendar date when it is a counterpart. The relative label keeps the existing footer ladder, which shows a numeric calendar date once a message is 7 days old or more.
- **Absolute short form:** The creation date and time in the short date and short time style of the user's regional format (see AC-UI-MESSAGE-TIME-DISPLAY-001.4).
- **Absolute long form:** The creation date and time in the long date style with a time that includes seconds, in the same regional format.
- **Counterpart form:** The one form that the message time label does not use, defined by AC-UI-MESSAGE-TIME-DISPLAY-001.3. The absolute long form is only ever a label, never a counterpart.

## Requirements

### REQ-UI-MESSAGE-TIME-DISPLAY-001: Message Time Display Preference

**Intent:** A user chooses whether transcript message timestamps read as a relative age, a short absolute date and time, or a long absolute date and time, and can still read the other form on demand.

**User story:** As a Kandev user, I want to pick how transcript message times are written, so that I can scan recent activity or audit exact moments without leaving the transcript.

#### Acceptance criteria

- **AC-UI-MESSAGE-TIME-DISPLAY-001.1:** The user can choose exactly one of **Relative**, **Absolute (short)**, or **Absolute (long)** as the message time display, in Settings under Preferences, Task behavior, on the Conversation tab. A settings search hit for the control opens that tab.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.2:** When a user has no saved choice, the system shall use **Relative**. The footer label is unchanged for users who never open the setting, except that the numeric calendar date shown at 7 days or more follows the date conventions of AC-UI-MESSAGE-TIME-DISPLAY-001.4 instead of the runtime default locale. The tooltip changes from the browser's default full date-time string to the absolute short form (AC-UI-MESSAGE-TIME-DISPLAY-001.3).
- **AC-UI-MESSAGE-TIME-DISPLAY-001.3:** The tooltip on a message time label shall show only the counterpart form: the absolute short form when the label is relative, and the relative form when the label is absolute short or absolute long. The tooltip shall not repeat the label text.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.4:** When the choice is **Absolute (short)** or **Absolute (long)**, the system shall render the message time label in that form and the form shall not change as time passes. Date and time conventions follow the interface locale when it names a region (for example `zh-HK` or `pt-PT`). When the interface locale names no region (for example `en`), they follow the first browser language preference whose base language matches, such as day-first dates for an English interface on an `en-GB` browser; when none matches, they follow the interface locale.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.5:** When a coarse-pointer device taps the message time label, the system shall open the existing touch drawer showing the same counterpart form that the tooltip shows on fine-pointer devices. The touch drawer trigger's accessible name shall contain both the visible label text and the counterpart form. On fine-pointer devices the label is the element's content and the counterpart is its tooltip.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.6:** When a message has no parseable creation time, the system shall render no message time label, no tooltip, and no drawer trigger, for every choice.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.7:** Changing the choice updates a local draft that shows the dirty state until the shared **Save changes** action succeeds. The shared **Reset** action discards the draft without a settings write, and a failed save keeps the draft so the user can retry.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.8:** When the choice is saved, the system shall persist it as a per-user setting, restore it after reload and on other devices, and apply it to open transcripts in other tabs without a reload. A PATCH that omits the field leaves the saved choice unchanged, and an unknown stored or submitted value is never accepted as a choice: unknown stored values read as **Relative** and an unknown submitted value is rejected.
- **AC-UI-MESSAGE-TIME-DISPLAY-001.9:** At a 390 px phone viewport, the control shall have a touch target of at least 44 px. For each timestamp form, the rendered label shall remain visible within the message wrapper, the footer's `scrollWidth` shall not exceed its `clientWidth`, and the document shall have no horizontal overflow.

## Out of scope

- Live re-rendering of the relative label as time passes. The label keeps its current refresh behavior.
- Timestamps outside the transcript message footer, including the sidebar, task lists, Office surfaces, and Account security "Last seen" (see [relative last seen](relative-last-seen.md)).
- Per-session or per-message overrides of the choice.
- A custom date pattern, 12-hour versus 24-hour override, or time zone selection.
- Changing which messages show a timestamp, or the footer layout and visibility rules.
