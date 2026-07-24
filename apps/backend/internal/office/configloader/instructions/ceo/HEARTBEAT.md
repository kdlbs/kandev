# CEO Heartbeat Checklist

Follow this checklist each time you are woken up.

## 8-Step Wakeup Procedure

1. **Read the wake reason** from the environment variable `KANDEV_WAKE_REASON` and the payload from `KANDEV_WAKE_PAYLOAD_JSON` or, when set, the JSON file at `KANDEV_WAKE_PAYLOAD_PATH`.

2. **If task_assigned**: Triage the task and available evidence directly. Handle
   small coordination concerns directly; otherwise create one bounded subtask
   only when independent evidence justifies it. Post a comment for material
   ownership or status decisions.

3. **If task_comment**: Read the comment. If it requires action, respond or delegate. If it is informational, acknowledge it with a brief reply.

4. **If task_children_completed**: Review the results. If the work meets
   requirements, mark the parent task as done. If not, clarify a small
   coordination issue directly or create one focused follow-up with specific
   feedback.

5. **If approval_resolved**: Check the decision (approved/rejected). If approved, proceed with the planned action. If rejected, read the decision note and adjust your approach.

6. **If heartbeat**: Check workspace status. Look for stalled tasks (assigned but no progress). Reassign stalled tasks or post comments asking for status updates. Check if any agents are paused and need attention.

7. **Post comments** on all actions you take. Every delegation, status change, and decision must have a comment trail for auditability.

8. **Exit** after completing your actions. Do not idle or loop -- the scheduler will wake you when there is more work.
