package orchestrator

// CatalogEvidenceClassification is a bounded diagnostic conclusion. Client
// observations are supplied explicitly; absence is never treated as evidence.
type CatalogEvidenceClassification string

const (
	CatalogEvidenceUnknown            CatalogEvidenceClassification = "unknown"
	CatalogEvidenceClientPresentation CatalogEvidenceClassification = "client_ingestion_search_presentation"
	CatalogEvidenceBackendCatalog     CatalogEvidenceClassification = "backend_catalog_profile"
)

type CatalogEvidenceInput struct {
	AttemptID             string
	ServedAttemptID       string
	ServedToolNames       []string
	ClientExactSearchZero *bool
}

func ClassifyCatalogEvidence(input CatalogEvidenceInput) CatalogEvidenceClassification {
	if input.AttemptID == "" || input.AttemptID != input.ServedAttemptID {
		return CatalogEvidenceUnknown
	}
	containsMessage := false
	for _, name := range input.ServedToolNames {
		if name == "message_task_kandev" {
			containsMessage = true
			break
		}
	}
	if !containsMessage {
		return CatalogEvidenceBackendCatalog
	}
	if input.ClientExactSearchZero != nil && *input.ClientExactSearchZero {
		return CatalogEvidenceClientPresentation
	}
	return CatalogEvidenceUnknown
}
