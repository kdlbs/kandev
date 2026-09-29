package types

import "context"

// UserInputOption is a choice offered by a protocol-native question.
type UserInputOption struct {
	OptionID    string
	Label       string
	Description string
}

// UserInputQuestion is a protocol-native question normalized for Kandev's
// clarification controls.
type UserInputQuestion struct {
	ID       string
	Header   string
	Prompt   string
	IsOther  bool
	IsSecret bool
	Options  []UserInputOption
}

// UserInputRequest identifies one provider question request. Provider thread,
// turn, and item IDs remain distinct from Kandev session and task IDs.
type UserInputRequest struct {
	ThreadID  string
	TurnID    string
	ItemID    string
	Questions []UserInputQuestion
}

// UserInputAnswer contains the selected offered options and optional free text.
type UserInputAnswer struct {
	OptionIDs  []string
	CustomText string
}

// UserInputResponse is the user's response to a protocol-native question set.
type UserInputResponse struct {
	Answers  map[string]UserInputAnswer
	Rejected bool
}

// UserInputRequestHandler creates a clarification request and waits for its
// response. Its context is cancelled when the provider resolves the request.
type UserInputRequestHandler func(context.Context, *UserInputRequest) (*UserInputResponse, error)
