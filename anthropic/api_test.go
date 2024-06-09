package anthropic

import (
	"net/http"
	"testing"

)

func TestCreateMessage(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY") 
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY environment variable is not set. Skipping test.")
	}

	client := &Client{
		APIKey:           apiKey,
		AnthropicVersion: "2023-06-01",
		HTTP:             &http.Client{},
	}

	req := &MessageRequest{
		Model: "claude-3-opus-20240229",
		MaxTokens: 1024,
		Messages: []Message{
			{
				Role:    "user",
				Content: []interface{}{map[string]string{"type": "text", "text": "Hello, world"}},
			},
		},
	}

	resp, err := client.CreateMessage(req)
	if err != nil {
		t.Fatalf("Failed to create message: %s", err)
	}

	if resp.Type != "message" {
		t.Errorf("Expected response type to be 'message', but got '%s'", resp.Type)
	}

	if resp.Role != "assistant" {
		t.Errorf("Expected response role to be 'assistant', but got '%s'", resp.Role)
	}

	if len(resp.Content) == 0 {
		t.Error("Response content is empty")
	}

	if resp.Model != "claude-3-opus-20240229" {
		t.Errorf("Expected response model to be 'claude-3-opus-20240229', but got '%s'", resp.Model)
	}

	if resp.StopReason != "end_turn" {
		t.Errorf("Expected response stop reason to be 'end_turn', but got '%s'", resp.StopReason)
	}

	if resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 {
		t.Error("Response usage tokens are zero")
	}
}
