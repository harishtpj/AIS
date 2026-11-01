package ontology

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

func LoadFile(path string) (*Document, error) {
	if path == "" {
		return nil, fmt.Errorf("ontology path cannot be empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ontology file %q: %w", path, err)
	}
	var document Document
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse ontology file %q: %w", path, err)
	}
	if err := document.Validate(); err != nil {
		return nil, fmt.Errorf("validate ontology file %q: %w", path, err)
	}
	return &document, nil
}
