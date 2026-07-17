package main

import "my-agent/internal/llm"

// Session represents a single chat conversation with independent history.
type Session struct {
	ID       int
	Name     string
	Messages []llm.Message
}

// NewSession creates a session with the given ID and display name.
func NewSession(id int, name string) *Session {
	return &Session{
		ID:       id,
		Name:     name,
		Messages: make([]llm.Message, 0),
	}
}

// AddMessage appends a message to the session's conversation history.
func (s *Session) AddMessage(msg llm.Message) {
	s.Messages = append(s.Messages, msg)
}
