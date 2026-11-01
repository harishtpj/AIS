package ontology

import "testing"

func TestDocumentValidateAppliesDocumentVersion(t *testing.T) {
	document := &Document{
		Version: "v1",
		Entities: []Entity{{
			Name:   "Item",
			Fields: map[string]Field{"name": {Type: "string"}},
		}},
	}

	if err := document.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := document.Entities[0].Version; got != "v1" {
		t.Fatalf("entity version = %q, want v1", got)
	}
}

func TestDocumentValidateRejectsDuplicateEntities(t *testing.T) {
	document := &Document{
		Version: "v1",
		Entities: []Entity{
			{Name: "Item", Fields: map[string]Field{"name": {Type: "string"}}},
			{Name: "Item", Fields: map[string]Field{"id": {Type: "string"}}},
		},
	}

	if err := document.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate entity error")
	}
}
