// Portable workflow YAML whose Review step keeps reviewers off the author's
// provider (exclude_author_provider). Shared by the author-exclusion specs.
export function authorExclusionWorkflowYaml(name: string): string {
  return `version: 2
type: kandev_workflow
workflows:
  - name: ${name}
    steps:
      - name: Work
        position: 0
        color: bg-blue-500
        stage_type: work
        is_start_step: true
        complete_task_on_enter: false
        events: {}
      - name: Review
        position: 1
        color: bg-yellow-500
        stage_type: review
        complete_task_on_enter: false
        events:
          on_enter:
            - type: ensure_participant_seat
              config:
                role: reviewer
                exclude_author_provider: true`;
}
