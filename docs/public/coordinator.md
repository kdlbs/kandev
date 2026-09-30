---
title: "Coordinator Autonomy"
description: "Let a workspace coordinator act on its own within a cost ceiling, and see when it is held, why, and how to clear it."
---

# Coordinator Autonomy

This page is for workspace managers who want a coordinator to act without someone starting each turn. It explains what autonomy does, how to turn it on, and how to read and clear a hold.

Autonomy is available when your administrator has enabled the coordinator features. If you do not see an **Autonomy** section on a coordinator's settings page, it is not on for this install.

## Quick path

1. Open **Settings**, choose the workspace, then the coordinator, then **Autonomy**.
2. Set a **Cost ceiling** in USD per 24 hours, from 0.01 to 10000.00.
3. Turn on **Let the coordinator act on its own** and save.
4. Check the **Containment** list. Every condition must read **Met** before the coordinator will run unattended.

With autonomy on, the coordinator wakes for new events, such as a task that needs an answer or a stalled task. It works within the permissions you set under **May do**. Anything that needs a manager still waits for one.

## Read the autonomy strip

The strip sits above the counts on **Needs you** and **Queue**. It shows the state, when the coordinator last woke, how many events are pending, and spend over the last 24 hours.

- **Active**: the coordinator will wake for the next event. A short note explains a pause, such as waiting for the conversation or the gap between turns.
- **Held**: something is stopping it. The reason is named in brackets, for example "Held (Cost ceiling reached)".
- **Off**: autonomy is turned off.
- **State unavailable**: Kandev could not read the state. Select **Try again**. Unavailable never means healthy.

If a turn was asked to stop at the ceiling and is still running after five minutes, the strip warns that the stop is not confirmed and offers **Stop**.

## Clear a hold

A hold with events waiting also appears as an item on **Needs you**, with an **Open settings** link for managers. The common reasons and what clears them:

| Reason | What clears it |
| --- | --- |
| Containment not in place | Fix the named condition. Each unmet condition in the settings list says how. |
| Spend cannot be measured | Some usage has no price or could not be read. It clears when the 24-hour window no longer holds it. |
| Cost ceiling reached | Raise the ceiling, or wait for spend to age out of the 24-hour window. |
| No conversation | Open the copilot once to give the coordinator a conversation. |
| Conversation stopped | Open the copilot to restart it. |

After a fix, select **Check again** in the Autonomy section to re-read the state.

## Containment

Autonomy runs without a person watching, so Kandev requires four conditions first:

- The coordinator's executor is isolated (Docker, remote Docker, Sprites, or Kubernetes).
- Kandev authentication is on.
- No Kandev credential is present in the executor or agent profile environment.
- The agent profile has no extra MCP servers.

## See what woke the coordinator

In the coordinator conversation, an unattended turn opens with a **Woken by N events** entry. Select **Show** to list the events, each with its kind, task, and title. The entry also shows how many permission requests were denied during the turn. If Kandev has pruned the record of an old turn, the entry reads "Woken by events" and shows the original message text behind **Show**.

## Related pages

- [Coordinate work](coordination.md)
- [Security](security.md)
