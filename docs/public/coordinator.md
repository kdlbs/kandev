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
- **Paused**: a manager paused the coordinator. The strip names who paused it and when, and takes precedence over Active and Held.
- **Off**: autonomy is turned off.
- **State unavailable**: Kandev could not read the state. Select **Try again**. Unavailable never means healthy.

If a turn was asked to stop at the ceiling and is still running after five minutes, the strip warns that the stop is not confirmed and offers **Stop**.

## Pause a coordinator

Pause stops a coordinator from acting on its own without touching its settings. Managers see **Pause** on the strip and in the **Autonomy** section, and **Resume** while it is paused. The change takes effect at once and needs no save.

- A turn the coordinator is running on its own is stopped, and its events return to the queue.
- No new unattended turn starts and no proposal is approved automatically. Such a proposal waits for a manager with the note "Paused; a manager will decide".
- Pending events are kept. After **Resume** they are delivered through the usual rules, including the five-minute gap between turns.
- Your own messages to the coordinator still start a turn.
- Turning autonomy off is different: it discards the queue. Pause keeps it.

Pausing works while autonomy is off, from the **Autonomy** section. If the phase 3.1 features are turned off on a paused coordinator, it stays paused and shows a read-only **Paused** badge until they are back on.

## Learn from its own turns

With the phase 3.1 features on, a coordinator's settings have a **Learning** section. It shows what the coordinator would change about itself. Shadow changes nothing: no suggestion is applied, copied or approved.

- **Shadow dream** is a per-coordinator switch, off by default. While it is on and the coordinator has run unattended turns, a short review runs about once a day. The review reads only the coordinator's own turn history and writes a report.
- **Health** reads Fresh, Waiting, Stale or Never run. A waiting dream names what is missing (for example, turn autonomy on) and how to fix it.
- **Measures** cover the last 7, 30 or 90 days: proposals approved without edits, how often an override came back, dollars per merged task, median wait for you, and how your ratings compare with replay. A measure with too little data says so instead of showing a number. An unknown cost shows as "unknown".
- **Reports** list the newest 20 with a status (clean, partial, failed or skipped), the turn and item counts and the cost. Select **Open** for the items, each with its gate result, replay result, the turns it cites, and a **Considered, not proposed** list that the agent reported and Kandev did not verify.
- Managers rate each item **Useful**, **Not useful** or **Harmful**. The newest rating wins. Readers see the section read-only.

Reports are kept for 400 days. The review never runs a tool other than reading the coordinator's turn history.

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

## Let it create tasks on its own

Under **May do**, a coordinator's **Create a card** permission can be raised from **Requires approval** to **Automatic**. Automatic is available for this permission only; every other action shows **Cannot be raised**. An automatic create makes one ordinary task and never starts an agent.

The **Automatic** option stays off until the coordinator has earned it. The settings list each condition as **Met** or **Not met** with its current value:

- Its first decided card is at least 30 days old.
- The last 30 days hold at least 20 decided cards.
- At least 90% of them were approved without edits.
- No task it created in the last 30 days was undone.
- You reviewed the last 30 days within the past 7 days.

Select **Review the last 30 days** to open the log filtered to this coordinator, then **Mark as reviewed**. To raise the permission, choose **Automatic** and save. The settings then show who raised it and when.

Automatic approvals appear in **What it did** with the authorization **Automatic** and the manager who raised the permission. At most 10 are approved in any 24 hours; further proposals wait for you. If an automatic approval fails, the proposal stays for you to approve, edit, or reject.

You can lower the permission back to **Requires approval** at any time. Undoing a task an automatic approval created lowers it for you.

## Choose the agent for created tasks

Each coordinator has an **Agent for created tasks**: an agent profile and an executor profile. Both are required, on the add form, in guided setup, and on the coordinator's settings page.

When you approve a card, the task it creates starts with this agent unless the card's board step, the board, or the workspace already names one. Automatic approvals work the same way. The card shows **Runs with** and the agent's name while it is pending or failed. If no agent is available, the card says so and points back to this setting.

An agent profile that uses CLI passthrough cannot be chosen here. If the profile or executor is later removed, the settings page warns you, and approving a card that needs it fails with an explanation and creates nothing. Pick another agent to fix it.

## Related pages

- [Coordinate work](coordination.md)
- [Security](security.md)
