package service

// Office pricing prioritizes provider-reported cost, models.dev rates, then
// estimated usage. internal/office/costs exposes the shared checked calculator
// and provider inference; internal/office/costs/modelsdev owns catalogue lookup.
