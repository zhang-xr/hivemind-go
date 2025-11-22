package history

import "agenthive-go/pkg/types"

type Strategy interface {
	Apply(messages []types.Message) []types.Message
}

type NoOpStrategy struct{}

func (s *NoOpStrategy) Apply(messages []types.Message) []types.Message {

	copiedMessages := make([]types.Message, len(messages))

	copy(copiedMessages, messages)
	return copiedMessages
}

type KeepLastN struct {
	N int
}

func NewKeepLastN(n int) *KeepLastN {
	return &KeepLastN{N: n}
}

func (s *KeepLastN) Apply(messages []types.Message) []types.Message {

	if s.N <= 0 || s.N >= len(messages) {
		copiedMessages := make([]types.Message, len(messages))
		copy(copiedMessages, messages)
		return copiedMessages
	}

	start := len(messages) - s.N
	truncated := messages[start:]

	copiedMessages := make([]types.Message, len(truncated))
	copy(copiedMessages, truncated)
	return copiedMessages
}
