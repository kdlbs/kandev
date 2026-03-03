package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
)

// runTUI starts a simple terminal UI mode for passthrough/PTY testing.
// It renders a prompt, processes input, shows a response, and waits for more input.
// The idle timeout in InteractiveRunner triggers turn completion when output stops.
func runTUI(model, initialPrompt string) {
	// Print header
	fmt.Print("\033[1;36m╭─ Mock Agent ─╮\033[0m\r\n")
	fmt.Print("\033[1;36m╰──────────────╯\033[0m\r\n\r\n")

	// Process initial prompt if provided via --prompt flag
	if initialPrompt != "" {
		fmt.Printf("\033[1;32m❯\033[0m %s\r\n", initialPrompt)
		processTUIPrompt(initialPrompt, model)
	}

	// Show ready prompt and wait for stdin
	fmt.Print("\033[1;32m❯\033[0m ")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			fmt.Print("\033[1;32m❯\033[0m ")
			continue
		}
		processTUIPrompt(line, model)
		fmt.Print("\033[1;32m❯\033[0m ")
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "mock-agent tui: scanner error: %v\n", err)
		os.Exit(1)
	}
}

// processTUIPrompt simulates agent processing: shows thinking, then a response.
func processTUIPrompt(prompt, model string) {
	// Show thinking state
	fmt.Print("\033[33m⠋ Thinking...\033[0m\r\n")
	fixedDelay(tuiDelay(model))

	// Show response
	fmt.Print("\r\n")
	fmt.Print("This is a simple mock response to your request.\r\n")
	if len(prompt) > 80 {
		prompt = prompt[:80] + "..."
	}
	fmt.Printf("Processed: %s\r\n\r\n", prompt)
}

// tuiDelay returns the thinking delay in milliseconds based on the model.
func tuiDelay(model string) int {
	switch model {
	case "mock-fast":
		return 100
	case "mock-slow":
		return 2000
	default:
		return 500
	}
}

// parseTUIFlag checks if --tui is present in the command line args.
func parseTUIFlag() bool {
	return slices.Contains(os.Args[1:], "--tui")
}

// parsePromptFlag extracts --prompt value from command line args.
func parsePromptFlag() string {
	return parsePromptFromArgs(os.Args)
}

// parsePromptFromArgs extracts --prompt value from the given args slice.
func parsePromptFromArgs(args []string) string {
	for i, arg := range args[1:] {
		if arg == "--prompt" && i+1 < len(args)-1 {
			return args[i+2]
		}
		if v, ok := strings.CutPrefix(arg, "--prompt="); ok {
			return v
		}
	}
	return ""
}
