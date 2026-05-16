package model

import "strings"

func NormalizeWireAPI(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return WireAPIResponses
	case "chat", "chat_completions", "chat-completions":
		return WireAPIChat
	case "responses":
		return WireAPIResponses
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func IsSupportedWireAPI(value string) bool {
	switch NormalizeWireAPI(value) {
	case WireAPIChat, WireAPIResponses:
		return true
	default:
		return false
	}
}
