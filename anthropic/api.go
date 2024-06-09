package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	BaseURL = "https://api.anthropic.com"
)

type Client struct {
	APIKey           string
	AnthropicVersion string
	HTTP             *http.Client
}

type Message struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type MessageRequest struct {
	Model           string     `json:"model"`
	Messages        []Message  `json:"messages"`
	MaxTokens       int        `json:"max_tokens"`
	Stream          bool       `json:"stream"`
//	ToolChoice      ToolChoice `json:"tool_choice,omitempty"`
	Temperature     float64    `json:"temperature,omitempty"`
//	TopK            int        `json:"top_k,omitempty"`
//	TopP            float64    `json:"top_p,omitempty"`
//	StopSequences   []string   `json:"stop_sequences,omitempty"`
	System          string     `json:"system,omitempty"`
//	MetaData        MetaData   `json:"meta_data,omitempty"`
//	Tools           []Tool     `json:"tools,omitempty"`
}

type ToolChoice struct {
	// Implement based on the nested object structure
}

type MetaData struct {
	// Implement based on the nested object structure 
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

func (c *Client) CreateMessage(req *MessageRequest) (*MessageResponse, error) {
	url := fmt.Sprintf("%s/v1/messages", BaseURL)
	
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", c.AnthropicVersion)
	
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		err = json.NewDecoder(resp.Body).Decode(&errResp)
		if err != nil {
			return nil, fmt.Errorf("API error: %s", resp.Status)
		}
		return nil, fmt.Errorf("API error: %d %s - %s",resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
	}
	
	var msgResp MessageResponse
	err = json.NewDecoder(resp.Body).Decode(&msgResp)
	if err != nil {
		return nil, err  
	}
	
	return &msgResp, nil
}

type MessageResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"` 
	Content    []interface{} `json:"content"`
	Model      string        `json:"model"`
	StopReason string        `json:"stop_reason"`
	Usage      Usage         `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
