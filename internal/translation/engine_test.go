package translation

import (
	"reflect"
	"testing"
)

func TestApplyBuildsNestedTarget(t *testing.T) {
	input := map[string]any{
		"item": map[string]any{
			"name":  "Laptop",
			"price": 1200,
		},
	}

	got, err := NewEngine().Apply(input, []Mapping{
		{From: "item.name", To: "details.title"},
		{From: "item.price", To: "pricing.amount"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := map[string]any{
		"details": map[string]any{"title": "Laptop"},
		"pricing": map[string]any{"amount": 1200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Apply() = %#v, want %#v", got, want)
	}
}

func TestApplySkipsMissingSource(t *testing.T) {
	got, err := NewEngine().Apply(map[string]any{"name": "Laptop"}, []Mapping{
		{From: "id", To: "productId"},
		{From: "name", To: "productName"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := map[string]any{"productName": "Laptop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Apply() = %#v, want %#v", got, want)
	}
}
