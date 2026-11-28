package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func sameHeldInference(a, b sessioncapacity.InferenceBinding) bool {
	return a.Key == b.Key && a.InstanceID == b.InstanceID && a.OwnerID == b.OwnerID && a.AttemptID == b.AttemptID && a.PreparationID == b.PreparationID
}

func matchesInferenceProof(held sessioncapacity.InferenceBinding, status *utility.AttemptStatus) bool {
	return definitelySettledInference(status) && status.InstanceID == held.InstanceID && status.OwnerID == held.OwnerID && status.AttemptID == held.AttemptID && status.PreparationID == held.PreparationID
}

func (b *sharedInferenceConsumer) retainInferenceTerminal(ctx context.Context, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) error {
	return b.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		held, ok := active[binding.Key]
		if !ok || !sameHeldInference(held, binding) || !matchesInferenceProof(held, status) {
			return fmt.Errorf("inference terminal owner changed")
		}
		held.TerminalState, held.TerminalPromptDispatch = status.State, status.PromptDispatch
		active[binding.Key] = held
		return nil
	})
}

func (b *sharedInferenceConsumer) removeSettledInference(ctx context.Context, binding sessioncapacity.InferenceBinding) (removeErr error) {
	defer func() {
		if removeErr == nil {
			b.logInferenceFlow(ctx, binding, "inference_settlement", "binding_removed", "", 0)
		}
	}()
	return b.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		held, ok := active[binding.Key]
		if !ok || !sameHeldInference(held, binding) || held.TerminalState != binding.TerminalState || held.TerminalPromptDispatch != binding.TerminalPromptDispatch {
			return fmt.Errorf("inference settled owner changed")
		}
		delete(active, binding.Key)
		return nil
	})
}

func (b *sharedInferenceConsumer) finishInferenceClaim(ctx context.Context, held sessioncapacity.InferenceBinding) error {
	if held.SettlementKind == inferenceSettlementNone && held.ProviderKey == "" {
		return nil
	}
	if b.settler == nil || !held.ProviderKnown || held.ProviderKey == "" {
		return fmt.Errorf("inference claim settlement unavailable")
	}
	return b.settler.FinishProviderTurn(ctx, held.Key, 1, held.TerminalPromptDispatch == utility.PromptNotSent)
}

func (b *sharedInferenceConsumer) validateInferenceSettlement(ctx context.Context, binding sessioncapacity.InferenceBinding) error {
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return err
	}
	held, ok := bindings[binding.Key]
	if !ok {
		return fmt.Errorf("inference admission owner unavailable")
	}
	if !sameHeldInference(held, binding) {
		return fmt.Errorf("inference admission owner changed")
	}
	if held.SettlementKind == inferenceSettlementNone && held.ProviderKey == "" {
		return nil
	}
	if b.settler == nil || !held.ProviderKnown || held.ProviderKey == "" {
		return fmt.Errorf("inference claim settlement unavailable")
	}
	return nil
}

func mergeObservedInference(active, original, observed map[string]sessioncapacity.InferenceBinding) error {
	for key, prior := range original {
		current, exists := active[key]
		if !exists {
			continue
		}
		if !sameHeldInference(current, prior) {
			return fmt.Errorf("inference observation owner changed")
		}
		if next, keep := observed[key]; keep {
			active[key] = next
		} else {
			delete(active, key)
		}
	}
	for key, next := range observed {
		if _, existed := original[key]; existed {
			continue
		}
		if current, exists := active[key]; exists {
			if !sameHeldInference(current, next) {
				return fmt.Errorf("inference imported owner changed")
			}
			continue
		}
		active[key] = next
	}
	return nil
}
