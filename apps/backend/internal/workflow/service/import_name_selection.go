package service

import "github.com/kandev/kandev/internal/workflow/models"

type workflowImportSelection struct {
	indexes []int
	skipped []string
}

func selectWorkflowImports(export *models.WorkflowExport, existingNames map[string]bool) workflowImportSelection {
	seen := make(map[string]bool, len(existingNames)+len(export.Workflows))
	for name, exists := range existingNames {
		seen[name] = exists
	}
	selection := workflowImportSelection{skipped: make([]string, 0)}
	for index, workflow := range export.Workflows {
		if seen[workflow.Name] {
			selection.skipped = append(selection.skipped, workflow.Name)
			continue
		}
		seen[workflow.Name] = true
		selection.indexes = append(selection.indexes, index)
	}
	return selection
}
