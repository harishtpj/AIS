package translation

import (
	"fmt"
	"strings"
)

type Mapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Plan struct {
	Service     string    `json:"service,omitempty"`
	Method      string    `json:"method,omitempty"`
	BackendPath string    `json:"backend_path,omitempty"`
	Request     []Mapping `json:"request"`
	Response    []Mapping `json:"response"`
}

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Apply(input map[string]any, mappings []Mapping) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("translation input cannot be nil")
	}
	output := make(map[string]any, len(mappings))
	for _, mapping := range mappings {
		from := strings.TrimSpace(mapping.From)
		to := strings.TrimSpace(mapping.To)
		if from == "" || to == "" {
			return nil, fmt.Errorf("translation mapping requires from and to paths")
		}
		value, ok, err := getPath(input, from)
		if err != nil {
			return nil, fmt.Errorf("read source path %q: %w", from, err)
		}
		if !ok {
			continue
		}
		if err := setPath(output, to, value); err != nil {
			return nil, fmt.Errorf("write target path %q: %w", to, err)
		}
	}
	return output, nil
}

func getPath(root map[string]any, path string) (any, bool, error) {
	parts, err := splitPath(path)
	if err != nil {
		return nil, false, err
	}
	var current any = root
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("path segment %q is not an object", part)
		}
		current, ok = object[part]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func setPath(root map[string]any, path string, value any) error {
	parts, err := splitPath(path)
	if err != nil {
		return err
	}
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("path segment %q is not an object", part)
		}
		current = child
	}
	current[parts[len(parts)-1]] = value
	return nil
}

func splitPath(path string) ([]string, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("path cannot be empty")
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, fmt.Errorf("path %q contains an empty segment", path)
		}
	}
	return parts, nil
}
