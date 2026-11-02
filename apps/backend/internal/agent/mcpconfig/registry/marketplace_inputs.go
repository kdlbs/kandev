package registry

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

const (
	argumentTypeNamed         = "named"
	unsupportedInputVariables = "input_variables_unsupported"
	unsupportedInputArguments = "input_arguments_unsupported"
)

func prepareChoiceInputs(choice Choice, request InstallRequest) (Choice, error) {
	if unsupportedChoiceInputs(choice) != "" {
		return Choice{}, ErrMarketplaceChoiceUnsupported
	}
	choice.EnvironmentVariables = append([]KeyValueInput(nil), choice.EnvironmentVariables...)
	choice.Headers = append([]KeyValueInput(nil), choice.Headers...)
	inputs, err := choiceInputMap(choice)
	if err != nil {
		return Choice{}, err
	}
	bound, err := applyChoiceSecretBindings(inputs, request.SecretBindings)
	if err != nil {
		return Choice{}, err
	}
	for name, value := range request.InputValues {
		input := inputs[name]
		if input == nil || input.IsSecret {
			return Choice{}, mcpconfig.ErrMCPInvalidDefinition
		}
		input.Value = value
	}
	for _, input := range inputs {
		if !input.IsRequired {
			continue
		}
		provided := strings.TrimSpace(input.Value) != ""
		if input.IsSecret {
			provided = bound[input.Name]
		}
		if !provided {
			return Choice{}, fmt.Errorf("%w: required setup input is missing", mcpconfig.ErrMCPInvalidDefinition)
		}
	}
	return choice, nil
}

func unsupportedChoiceInputs(choice Choice) string {
	if len(choice.Variables) > 0 {
		return unsupportedInputVariables
	}
	for _, group := range [][]KeyValueInput{choice.EnvironmentVariables, choice.Headers} {
		for _, input := range group {
			if len(input.Variables) > 0 {
				return unsupportedInputVariables
			}
		}
	}
	for _, group := range [][]Argument{choice.RuntimeArguments, choice.PackageArguments} {
		for _, argument := range group {
			if len(argument.Variables) > 0 {
				return unsupportedInputVariables
			}
			if !fixedArgumentSupported(argument) {
				return unsupportedInputArguments
			}
		}
	}
	return ""
}

func fixedArgumentSupported(argument Argument) bool {
	if argument.IsSecret {
		return false
	}
	switch argument.Type {
	case "", argumentTypeNamed, "positional":
	default:
		return false
	}
	if argument.Type == argumentTypeNamed && strings.TrimSpace(argument.Name) == "" {
		return false
	}
	needsValue := argument.IsRequired || argument.Type != "" || argument.Format != ""
	return !needsValue || strings.TrimSpace(argument.Value) != ""
}

func choiceInputMap(choice Choice) (map[string]*KeyValueInput, error) {
	inputs := make(map[string]*KeyValueInput)
	for _, group := range [][]KeyValueInput{choice.EnvironmentVariables, choice.Headers} {
		for index := range group {
			input := &group[index]
			if input.Name == "" {
				return nil, mcpconfig.ErrMCPInvalidDefinition
			}
			if _, duplicate := inputs[input.Name]; duplicate {
				return nil, mcpconfig.ErrMCPInvalidDefinition
			}
			inputs[input.Name] = input
		}
	}

	return inputs, nil
}

func applyChoiceSecretBindings(inputs map[string]*KeyValueInput, bindings []SecretBindingInput) (map[string]bool, error) {
	bound := make(map[string]bool)
	for _, binding := range bindings {
		input := inputs[binding.InputName]
		if input == nil || strings.TrimSpace(binding.SecretID) == "" || bound[binding.InputName] {
			return nil, mcpconfig.ErrMCPInvalidDefinition
		}
		bound[binding.InputName] = true
		input.IsSecret = true
		input.Value = ""
	}
	return bound, nil
}
