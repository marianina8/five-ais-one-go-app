package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Bedrock talks to Amazon Bedrock's Converse API, which gives every Bedrock model the same
// request shape. AWS credentials come from the usual places: the environment, ~/.aws, or
// the role GitHub Actions signs in to.
type Bedrock struct {
	modelID   string
	maxTokens int32
	client    *bedrockruntime.Client
}

func NewBedrock(ctx context.Context, region, modelID string, maxTokens int) (*Bedrock, error) {
	if maxTokens <= 0 || maxTokens > math.MaxInt32 {
		return nil, fmt.Errorf("bedrock: max tokens %d out of range", maxTokens)
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithRetryMaxAttempts(6))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return &Bedrock{modelID: modelID, maxTokens: int32(maxTokens), client: bedrockruntime.NewFromConfig(cfg)}, nil // #nosec G115 -- range checked above
}

func (b *Bedrock) Chat(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	input := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(b.modelID),
		System:          []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: system}},
		InferenceConfig: &types.InferenceConfiguration{MaxTokens: aws.Int32(b.maxTokens)},
		ToolConfig:      bedrockTools(tools),
	}
	for _, msg := range messages {
		input.Messages = append(input.Messages, toBedrockMessage(msg))
	}

	out, err := b.client.Converse(ctx, input)
	if err != nil {
		return Response{}, fmt.Errorf("bedrock: %w", err)
	}
	reply, err := fromBedrockOutput(out.Output)
	if err != nil {
		return Response{}, err
	}
	var usage Usage
	if out.Usage != nil {
		usage = Usage{
			InputTokens:  int(aws.ToInt32(out.Usage.InputTokens)),
			OutputTokens: int(aws.ToInt32(out.Usage.OutputTokens)),
			CachedTokens: int(aws.ToInt32(out.Usage.CacheReadInputTokens)),
		}
	}
	return Response{Message: reply, Usage: usage, StopReason: string(out.StopReason)}, nil
}

func bedrockTools(tools []Tool) *types.ToolConfiguration {
	if len(tools) == 0 {
		return nil
	}
	toolConfig := &types.ToolConfiguration{}
	for _, tool := range tools {
		toolConfig.Tools = append(toolConfig.Tools, &types.ToolMemberToolSpec{Value: types.ToolSpecification{
			Name:        aws.String(tool.Name),
			Description: aws.String(tool.Description),
			InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(tool.Schema)},
		}})
	}
	return toolConfig
}

func fromBedrockOutput(output types.ConverseOutput) (Message, error) {
	reply := Message{Role: "assistant"}
	msg, ok := output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return reply, nil
	}
	for _, block := range msg.Value.Content {
		switch block := block.(type) {
		case *types.ContentBlockMemberText:
			reply.Text += block.Value
		case *types.ContentBlockMemberToolUse:
			var args map[string]any
			if err := block.Value.Input.UnmarshalSmithyDocument(&args); err != nil || args == nil {
				args = map[string]any{}
			}
			raw, err := json.Marshal(args)
			if err != nil {
				return reply, fmt.Errorf("bedrock: tool arguments: %w", err)
			}
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				ID:    aws.ToString(block.Value.ToolUseId),
				Name:  aws.ToString(block.Value.Name),
				Input: raw,
			})
		}
	}
	return reply, nil
}

// toBedrockMessage turns one of our messages into Converse content blocks.
func toBedrockMessage(msg Message) types.Message {
	out := types.Message{Role: types.ConversationRole(msg.Role)}
	for _, result := range msg.ToolResults {
		status := types.ToolResultStatusSuccess
		if result.IsError {
			status = types.ToolResultStatusError
		}
		out.Content = append(out.Content, &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
			ToolUseId: aws.String(result.CallID),
			Content:   []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: result.Output}},
			Status:    status,
		}})
	}
	if msg.Text != "" {
		out.Content = append(out.Content, &types.ContentBlockMemberText{Value: msg.Text})
	}
	for _, call := range msg.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal(call.Input, &args); err != nil || args == nil {
			args = map[string]any{}
		}
		out.Content = append(out.Content, &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String(call.ID),
			Name:      aws.String(call.Name),
			Input:     document.NewLazyDocument(args),
		}})
	}
	return out
}
