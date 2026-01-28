package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/invopop/jsonschema"
)

type APIProvider struct {
	BaseURL      string
	APIKey       string
	Model        string
	Headers      map[string]string
	ProviderType string // "openrouter" or "anthropic"
}

func getAPIProvider() (*APIProvider, error) {
	// Priority 1: OpenRouter
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		return &APIProvider{
			BaseURL:      "https://openrouter.ai/api/v1/chat/completions",
			APIKey:       apiKey,
			Model:        "anthropic/claude-sonnet-4.5",
			ProviderType: "openrouter",
			Headers: map[string]string{
				"HTTP-Referer": "https://praktor.ai",
				"X-Title":      "Praktor",
			},
		}, nil
	}

	// Priority 2: Anthropic API
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("neither OPENROUTER_API_KEY nor ANTHROPIC_API_KEY environment variable is set")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1/messages"
	} else {
		// Ensure custom base URL ends with /v1/messages
		if !strings.HasSuffix(baseURL, "/v1/messages") {
			baseURL = strings.TrimSuffix(baseURL, "/") + "/v1/messages"
		}
	}

	return &APIProvider{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        "claude-sonnet-4-20250514",
		ProviderType: "anthropic",
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	}, nil
}

func main() {
	provider, err := getAPIProvider()
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	tools := []ToolDefinition{
		ReadFileDefinition,
		ListFilesDefinition,
		EditFileDefinition,
	}

	agent := NewAgent(provider, getUserMessage, tools)
	err = agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(
	provider *APIProvider,
	getUserMessage func() (string, bool),
	tools []ToolDefinition,
) *Agent {
	return &Agent{
		provider:       provider,
		getUserMessage: getUserMessage,
		tools:          tools,
		client:         &http.Client{},
	}
}

type Agent struct {
	provider       *APIProvider
	getUserMessage func() (string, bool)
	tools          []ToolDefinition
	client         *http.Client
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    interface{} `json:"code"`
	} `json:"error,omitempty"`
}

type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Tools     []ToolDef     `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []ChatMessage{}

	apiName := "OpenRouter"
	if a.provider.ProviderType == "anthropic" {
		apiName = "Anthropic"
	}
	fmt.Printf("Chat with Praktor powered by %s (use 'ctrl-c' to quit)\n", apiName)

	readUserInput := true
	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := a.getUserMessage()
			if !ok {
				break
			}

			conversation = append(conversation, ChatMessage{
				Role:    "user",
				Content: userInput,
			})
		}

		toolCalls, responseText, err := a.runInference(ctx, conversation)
		if err != nil {
			return err
		}

		if responseText != "" {
			fmt.Printf("\u001b[93mPraktor\u001b[0m: %s\n", responseText)
			conversation = append(conversation, ChatMessage{
				Role:    "assistant",
				Content: responseText,
			})
		}

		if len(toolCalls) == 0 {
			readUserInput = true
			continue
		}

		// Add assistant message with tool calls to conversation
		asstMsgBytes, _ := json.Marshal(struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		}{
			Role:      "assistant",
			Content:   responseText,
			ToolCalls: toolCalls,
		})
		var asstMsgParsed ChatMessage
		json.Unmarshal(asstMsgBytes, &asstMsgParsed)
		conversation = append(conversation, asstMsgParsed)

		// Execute tools
		for _, toolCall := range toolCalls {
			result := a.executeTool(toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)
			conversation = append(conversation, ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}

		readUserInput = false
	}

	return nil
}

func (a *Agent) runInference(ctx context.Context, conversation []ChatMessage) ([]ToolCall, string, error) {
	isAnthropic := a.provider.ProviderType == "anthropic"

	var reqBody []byte
	var err error

	if isAnthropic {
		// Anthropic format
		reqBody, err = a.buildAnthropicRequest(conversation)
	} else {
		// OpenRouter/OpenAI format
		reqBody, err = a.buildOpenRouterRequest(conversation)
	}

	if err != nil {
		return nil, "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.provider.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if isAnthropic {
		httpReq.Header.Set("x-api-key", a.provider.APIKey)
		for k, v := range a.provider.Headers {
			httpReq.Header.Set(k, v)
		}
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+a.provider.APIKey)
		for k, v := range a.provider.Headers {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API error: %s", string(body))
	}

	if isAnthropic {
		return a.parseAnthropicResponse(body)
	}
	return a.parseOpenRouterResponse(body)
}

func (a *Agent) buildOpenRouterRequest(conversation []ChatMessage) ([]byte, error) {
	tools := []ToolDef{}
	for _, tool := range a.tools {
		params := map[string]interface{}{
			"type":       "object",
			"properties": tool.InputSchema.Properties,
		}
		td := ToolDef{
			Type: "function",
		}
		td.Function.Name = tool.Name
		td.Function.Description = tool.Description
		td.Function.Parameters = params
		tools = append(tools, td)
	}

	req := ChatRequest{
		Model:     a.provider.Model,
		Messages:  conversation,
		Tools:     tools,
		MaxTokens: 4096,
	}

	return json.Marshal(req)
}

func (a *Agent) buildAnthropicRequest(conversation []ChatMessage) ([]byte, error) {
	// Anthropic uses a different message format
	type AnthropicMessage struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	type AnthropicToolDef struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		InputSchema map[string]interface{} `json:"input_schema"`
	}

	type AnthropicRequest struct {
		Model     string               `json:"model"`
		Messages  []AnthropicMessage   `json:"messages"`
		Tools     []AnthropicToolDef   `json:"tools,omitempty"`
		MaxTokens int                  `json:"max_tokens,omitempty"`
	}

	messages := []AnthropicMessage{}
	for _, msg := range conversation {
		if msg.Role == "tool" {
			// Anthropic uses "user" role for tool responses with specific format
			messages = append(messages, AnthropicMessage{
				Role: "user",
				Content: map[string]interface{}{
					"type":      "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":   msg.Content,
				},
			})
		} else if len(msg.ToolCalls) > 0 {
			// Assistant message with tool calls
			blocks := []map[string]interface{}{
				{"type": "text", "text": msg.Content},
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
				blocks = append(blocks, map[string]interface{}{
					"type":        "tool_use",
					"id":          tc.ID,
					"name":        tc.Function.Name,
					"input":       args,
				})
			}
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
		} else {
			messages = append(messages, AnthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	tools := []AnthropicToolDef{}
	for _, tool := range a.tools {
		td := AnthropicToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": tool.InputSchema.Properties,
			},
		}
		tools = append(tools, td)
	}

	req := AnthropicRequest{
		Model:     a.provider.Model,
		Messages:  messages,
		Tools:     tools,
		MaxTokens: 4096,
	}

	return json.Marshal(req)
}

func (a *Agent) parseOpenRouterResponse(body []byte) ([]ToolCall, string, error) {
	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, "", err
	}

	if response.Error != nil {
		return nil, "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return nil, "", fmt.Errorf("no choices in response")
	}

	choice := response.Choices[0]
	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = choice.Message.ToolCalls
	}

	return toolCalls, choice.Message.Content, nil
}

func (a *Agent) parseAnthropicResponse(body []byte) ([]ToolCall, string, error) {
	type AnthropicContentBlock struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"`
	}

	type AnthropicResponse struct {
		ID      string `json:"id"`
		Content []AnthropicContentBlock `json:"content"`
		Error   *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error,omitempty"`
	}

	var response AnthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, "", err
	}

	if response.Error != nil {
		return nil, "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	var toolCalls []ToolCall
	var textContent strings.Builder

	for _, block := range response.Content {
		if block.Type == "text" {
			textContent.WriteString(block.Text)
		} else if block.Type == "tool_use" {
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	return toolCalls, textContent.String(), nil
}

func (a *Agent) executeTool(id, name string, arguments string) string {
	var toolDef ToolDefinition
	var found bool
	for _, tool := range a.tools {
		if tool.Name == name {
			toolDef = tool
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("Error: tool not found")
	}

	fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", name, arguments)
	response, err := toolDef.Function([]byte(arguments))
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error())
	}
	return response
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema ToolInputSchema
	Function    func(input []byte) (string, error)
}

type ToolInputSchema struct {
	Properties interface{}
}

var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	InputSchema: ReadFileInputSchema,
	Function:    ReadFile,
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

var ReadFileInputSchema = GenerateSchema[ReadFileInput]()

func ReadFile(input []byte) (string, error) {
	readFileInput := ReadFileInput{}
	err := json.Unmarshal(input, &readFileInput)
	if err != nil {
		panic(err)
	}

	content, err := os.ReadFile(readFileInput.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

var ListFilesDefinition = ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
	InputSchema: ListFilesInputSchema,
	Function:    ListFiles,
}

type ListFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from. Defaults to current directory if not provided."`
}

var ListFilesInputSchema = GenerateSchema[ListFilesInput]()

func ListFiles(input []byte) (string, error) {
	listFilesInput := ListFilesInput{}
	// Handle empty input or "{}"
	if len(input) > 0 && !bytes.Equal(input, []byte("{}")) {
		err := json.Unmarshal(input, &listFilesInput)
		if err != nil {
			return "", err
		}
	}

	dir := "."
	if listFilesInput.Path != "" {
		dir = listFilesInput.Path
	}

	var files []string
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if relPath != "." {
			if info.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if walkErr != nil {
		return "", walkErr
	}

	result, err := json.Marshal(files)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

var EditFileDefinition = ToolDefinition{
	Name: "edit_file",
	Description: `Make edits to a text file.

Replaces 'old_str' with 'new_str' in the given file. 'old_str' and 'new_str' MUST be different from each other.

If the file specified with path doesn't exist, it will be created.
`,
	InputSchema: EditFileInputSchema,
	Function:    EditFile,
}

type EditFileInput struct {
	Path   string `json:"path" jsonschema_description:"The path to the file"`
	OldStr string `json:"old_str" jsonschema_description:"Text to search for - must match exactly and must only have one match exactly"`
	NewStr string `json:"new_str" jsonschema_description:"Text to replace old_str with"`
}

var EditFileInputSchema = GenerateSchema[EditFileInput]()

func EditFile(input []byte) (string, error) {
	editFileInput := EditFileInput{}
	err := json.Unmarshal(input, &editFileInput)
	if err != nil {
		return "", err
	}

	if editFileInput.Path == "" || editFileInput.OldStr == editFileInput.NewStr {
		return "", fmt.Errorf("invalid input parameters")
	}

	content, err := os.ReadFile(editFileInput.Path)
	if err != nil {
		if os.IsNotExist(err) && editFileInput.OldStr == "" {
			return createNewFile(editFileInput.Path, editFileInput.NewStr)
		}
		return "", err
	}

	oldContent := string(content)
	newContent := strings.Replace(oldContent, editFileInput.OldStr, editFileInput.NewStr, -1)

	if oldContent == newContent && editFileInput.OldStr != "" {
		return "", fmt.Errorf("old_str not found in file")
	}

	err = os.WriteFile(editFileInput.Path, []byte(newContent), 0644)
	if err != nil {
		return "", err
	}

	return "OK", nil
}

func createNewFile(filePath, content string) (string, error) {
	dir := path.Dir(filePath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	return fmt.Sprintf("Successfully created file %s", filePath), nil
}

func GenerateSchema[T any]() ToolInputSchema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T

	schema := reflector.Reflect(v)

	return ToolInputSchema{
		Properties: schema.Properties,
	}
}
