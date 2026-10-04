package service

// Office pricing uses provider-reported cost first, then models.dev rates.
// Missing rates or overflow produce an unpriced result. Estimated describes
// token-count authority independently of cost source.
// internal/office/costs exposes the shared checked calculator
// and provider inference; internal/office/costs/modelsdev owns catalogue lookup.
