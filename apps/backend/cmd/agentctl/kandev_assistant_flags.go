package main

import (
	"flag"
	"os"
	"strconv"
)

type assistantMutationFlags struct {
	operation             *string
	intent                *int64
	objective, contextRef *string
}

func registerAssistantMutation(fs *flag.FlagSet) assistantMutationFlags {
	revision, err := strconv.ParseInt(os.Getenv("KANDEV_INTENT_REVISION"), 10, 64)
	if err != nil {
		revision = -1
	}
	return assistantMutationFlags{
		operation:  fs.String("operation-id", "", "Stable operation ID; reuse only for an identical retry"),
		intent:     fs.Int64("intent-revision", revision, "Expected user intent revision (injected for assistant runs)"),
		objective:  fs.String("objective", "", "Existing assistant objective ID"),
		contextRef: fs.String("context", "", "Versioned assistant context reference"),
	}
}

func (f assistantMutationFlags) add(payload map[string]any) {
	if *f.operation != "" {
		payload["operation_id"] = *f.operation
	}
	if *f.intent >= 0 {
		payload["expected_intent_revision"] = *f.intent
	}
	if *f.objective != "" {
		payload["objective_id"] = *f.objective
	}
	if *f.contextRef != "" {
		payload["context_ref"] = *f.contextRef
	}
}
