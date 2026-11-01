package slm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Candidate struct {
	CanonicalPath        string `json:"canonical_path"`
	CanonicalType        string `json:"canonical_type"`
	CanonicalFormat      string `json:"canonical_format,omitempty"`
	CanonicalDescription string `json:"canonical_description,omitempty"`
	BackendPath          string `json:"backend_path"`
	BackendType          string `json:"backend_type"`
	BackendFormat        string `json:"backend_format,omitempty"`
	BackendDescription   string `json:"backend_description,omitempty"`
}

type Suggestion struct {
	CanonicalPath  string  `json:"canonical_path"`
	BackendPath    string  `json:"backend_path"`
	Match          bool    `json:"match"`
	Confidence     float64 `json:"confidence"`
	Transformation string  `json:"transformation,omitempty"`
	Reason         string  `json:"reason,omitempty"`
}

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Format  string         `json:"format"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type generateResponse struct {
	Response string `json:"response"`
}

func NewOllamaClient(baseURL, model string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	if baseURL == "" {
		return nil, fmt.Errorf("Ollama URL cannot be empty")
	}
	if model == "" {
		return nil, fmt.Errorf("Ollama model cannot be empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) SuggestMappings(ctx context.Context, candidates []Candidate) ([]Suggestion, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("Ollama client is not initialized")
	}
	if len(candidates) == 0 {
		return []Suggestion{}, nil
	}

	input, err := json.Marshal(candidates)
	if err != nil {
		return nil, fmt.Errorf("encode mapping candidates: %w", err)
	}
	prompt := fmt.Sprintf(`You are an API schema mapping assistant.
Compare canonical fields with backend fields and return only valid JSON.
Do not invent fields. A match must represent the same business meaning and be
type-compatible. Use "identity" when no conversion is needed. Use a short
transformation name only when a deterministic conversion is required.

Return exactly this JSON shape:
{"suggestions":[{"canonical_path":"string","backend_path":"string","match":true,"confidence":0.0,"transformation":"identity","reason":"string"}]}

Candidates:
%s`, input)

	body, err := json.Marshal(generateRequest{
		Model:   c.model,
		Prompt:  prompt,
		Format:  "json",
		Stream:  false,
		Options: map[string]any{"temperature": 0},
	})
	if err != nil {
		return nil, fmt.Errorf("encode Ollama request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Ollama request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Ollama: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
	}

	var generated generateResponse
	if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
		return nil, fmt.Errorf("decode Ollama response: %w", err)
	}
	var result struct {
		Suggestions []Suggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(generated.Response), &result); err != nil {
		return nil, fmt.Errorf("decode Ollama JSON output: %w", err)
	}
	if err := validateSuggestions(candidates, result.Suggestions); err != nil {
		return nil, err
	}
	return result.Suggestions, nil
}

func validateSuggestions(candidates []Candidate, suggestions []Suggestion) error {
	allowed := make(map[string]map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.CanonicalPath == "" || candidate.BackendPath == "" {
			return fmt.Errorf("mapping candidates require canonical and backend paths")
		}
		if allowed[candidate.CanonicalPath] == nil {
			allowed[candidate.CanonicalPath] = make(map[string]struct{})
		}
		allowed[candidate.CanonicalPath][candidate.BackendPath] = struct{}{}
	}
	for _, suggestion := range suggestions {
		if suggestion.CanonicalPath == "" || suggestion.BackendPath == "" {
			return fmt.Errorf("Ollama returned a mapping with an empty path")
		}
		if _, ok := allowed[suggestion.CanonicalPath][suggestion.BackendPath]; !ok {
			return fmt.Errorf("Ollama returned an unknown mapping %q -> %q", suggestion.CanonicalPath, suggestion.BackendPath)
		}
		if suggestion.Confidence < 0 || suggestion.Confidence > 1 {
			return fmt.Errorf("Ollama returned invalid confidence for %q -> %q", suggestion.CanonicalPath, suggestion.BackendPath)
		}
	}
	return nil
}
