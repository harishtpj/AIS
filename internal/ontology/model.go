package ontology

import (
	"fmt"
	"strings"
)

type Document struct {
	Version  string   `yaml:"version" json:"version"`
	Entities []Entity `yaml:"entities" json:"entities"`
}

type Entity struct {
	Name    string           `yaml:"name" json:"name"`
	Version string           `yaml:"version,omitempty" json:"version,omitempty"`
	Fields  map[string]Field `yaml:"fields" json:"fields"`
}

type Field struct {
	Type        string `yaml:"type" json:"type"`
	Format      string `yaml:"format,omitempty" json:"format,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Identity    bool   `yaml:"identity,omitempty" json:"identity,omitempty"`
	Items       string `yaml:"items,omitempty" json:"items,omitempty"`
}

func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("ontology document cannot be nil")
	}
	d.Version = strings.TrimSpace(d.Version)
	if d.Version == "" {
		return fmt.Errorf("ontology document version is required")
	}
	if len(d.Entities) == 0 {
		return fmt.Errorf("ontology document must define at least one entity")
	}

	seen := make(map[string]struct{}, len(d.Entities))
	for i := range d.Entities {
		entity := &d.Entities[i]
		entity.Name = strings.TrimSpace(entity.Name)
		if entity.Name == "" {
			return fmt.Errorf("entity %d has no name", i)
		}
		if _, exists := seen[entity.Name]; exists {
			return fmt.Errorf("duplicate entity %q", entity.Name)
		}
		seen[entity.Name] = struct{}{}
		if entity.Version == "" {
			entity.Version = d.Version
		}
		if len(entity.Fields) == 0 {
			return fmt.Errorf("entity %q must define at least one field", entity.Name)
		}
		for name, field := range entity.Fields {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("entity %q contains an empty field name", entity.Name)
			}
			if strings.TrimSpace(field.Type) == "" {
				return fmt.Errorf("entity %q field %q has no type", entity.Name, name)
			}
		}
	}
	return nil
}
