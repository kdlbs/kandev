You have been assigned a Sentry issue to investigate.

**Issue:** {{issue.url}}
**Short ID:** {{issue.short_id}}
**Title:** {{issue.title}}
**Project:** {{issue.project}}
**Level:** {{issue.level}}
**Status:** {{issue.status}}
**Culprit:** {{issue.culprit}}
**Assignee:** {{issue.assignee}}
**Events:** {{issue.count}} (affecting {{issue.user_count}} users)
**First seen:** {{issue.first_seen}}
**Last seen:** {{issue.last_seen}}

## Instructions

1. Read the Sentry issue carefully and understand the failure mode.
2. Reproduce the problem locally where possible, or trace the culprit in the codebase.
3. Implement the fix.
4. Write or update tests covering the regression.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing {{issue.short_id}}.
