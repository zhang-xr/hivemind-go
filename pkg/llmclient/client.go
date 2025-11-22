package llmclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sashabaranov/go-openai"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	Content string
	Usage   openai.Usage
}

type LLMClient struct {
	config  *AppConfig
	clients map[string]*openai.Client
}

func NewLLMClient(config *AppConfig) *LLMClient {
	clients := make(map[string]*openai.Client)

	for name, providerConf := range config.Providers {

		clientConfig := openai.DefaultConfig(providerConf.APIKey)

		if providerConf.BaseURL != "" {
			clientConfig.BaseURL = providerConf.BaseURL
		}

		if providerConf.OrgID != "" {
			clientConfig.OrgID = providerConf.OrgID
		}

		clients[name] = openai.NewClientWithConfig(clientConfig)
	}
	return &LLMClient{
		config:  config,
		clients: clients,
	}
}

func (c *LLMClient) Invoke(ctx context.Context, messages []Message, maxRetries int) (*Response, error) {

	providerName := c.config.Common.ActiveModel
	providerConf, ok := c.config.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("active LLM provider '%s' not found in configuration", providerName)
	}
	client, ok := c.clients[providerName]
	if !ok {
		return nil, fmt.Errorf("client for provider '%s' not initialized", providerName)
	}

	apiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		apiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	var resp openai.ChatCompletionResponse
	var err error

	for i := 0; i < maxRetries; i++ {
		req := openai.ChatCompletionRequest{
			Model:       providerConf.Model,
			Messages:    apiMessages,
			Temperature: float32(providerConf.Temperature),
		}

		resp, err = client.CreateChatCompletion(ctx, req)

		if err == nil {
			return &Response{
				Content: resp.Choices[0].Message.Content,
				Usage:   resp.Usage,
			}, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if apiErr, ok := err.(*openai.APIError); ok {
			if apiErr.HTTPStatusCode >= http.StatusBadRequest && apiErr.HTTPStatusCode < http.StatusInternalServerError {
				return nil, fmt.Errorf("unrecoverable API error: %w", err)
			}
		}

		sleepDuration := time.Second * time.Duration(2<<i)
		fmt.Printf("LLM request failed (attempt %d/%d): %v. Retrying in %v...\n", i+1, maxRetries, err, sleepDuration)

		time.Sleep(sleepDuration)
	}

	return nil, fmt.Errorf("failed to get response from LLM after %d retries: %w", maxRetries, err)
}
