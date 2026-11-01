package translation

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pb33f/libopenapi"
	"go.yaml.in/yaml/v3"
)

type backendOperation struct {
	Method   string
	Path     string
	Request  []schemaField
	Response []schemaField
}

type schemaField struct {
	Path        string
	Type        string
	Format      string
	Description string
}

func parseOpenAPI(raw []byte) ([]backendOperation, error) {
	document, err := libopenapi.NewDocument(raw)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	if _, err := document.BuildV3Model(); err != nil {
		if _, v2Err := document.BuildV2Model(); v2Err != nil {
			return nil, fmt.Errorf("build OpenAPI model: %v; OpenAPI 2 fallback: %v", err, v2Err)
		}
	}

	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode OpenAPI document: %w", err)
	}
	paths, ok := root["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI document has no paths")
	}
	components, _ := root["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	var operations []backendOperation
	for path, rawPath := range paths {
		pathItem, ok := rawPath.(map[string]any)
		if !ok {
			continue
		}
		for _, method := range []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete,
		} {
			rawOperation, ok := pathItem[strings.ToLower(method)].(map[string]any)
			if !ok {
				continue
			}
			operation := backendOperation{Method: method, Path: path}
			operation.Request = extractBodyFields(rawOperation["requestBody"], schemas)
			operation.Response = extractResponseFields(rawOperation["responses"], schemas)
			if len(operation.Request) > 0 || len(operation.Response) > 0 {
				operations = append(operations, operation)
			}
		}
	}
	return operations, nil
}

func extractBodyFields(raw any, schemas map[string]any) []schemaField {
	body, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	content, _ := body["content"].(map[string]any)
	jsonContent, _ := content["application/json"].(map[string]any)
	return extractSchemaFields(jsonContent["schema"], schemas, "")
}

func extractResponseFields(raw any, schemas map[string]any) []schemaField {
	responses, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	for _, status := range []string{"200", "201", "202", "default"} {
		response, ok := responses[status].(map[string]any)
		if !ok {
			continue
		}
		content, _ := response["content"].(map[string]any)
		jsonContent, _ := content["application/json"].(map[string]any)
		fields := extractSchemaFields(jsonContent["schema"], schemas, "")
		if len(fields) > 0 {
			return fields
		}
	}
	return nil
}

func extractSchemaFields(raw any, schemas map[string]any, prefix string) []schemaField {
	schema, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := schema["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if resolved, exists := schemas[name]; exists {
			return extractSchemaFields(resolved, schemas, prefix)
		}
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		var fields []schemaField
		for _, part := range allOf {
			fields = append(fields, extractSchemaFields(part, schemas, prefix)...)
		}
		return fields
	}
	properties, _ := schema["properties"].(map[string]any)
	if typ, _ := schema["type"].(string); typ == "array" {
		return extractSchemaFields(schema["items"], schemas, prefix)
	}
	if len(properties) == 0 {
		return nil
	}
	required := make(map[string]bool)
	if values, ok := schema["required"].([]any); ok {
		for _, value := range values {
			if name, ok := value.(string); ok {
				required[name] = true
			}
		}
	}
	var fields []schemaField
	for name, rawProperty := range properties {
		property, _ := rawProperty.(map[string]any)
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		typ, _ := property["type"].(string)
		format, _ := property["format"].(string)
		description, _ := property["description"].(string)
		if typ == "object" || property["properties"] != nil || property["$ref"] != nil {
			nested := extractSchemaFields(property, schemas, path)
			if len(nested) > 0 {
				fields = append(fields, nested...)
				continue
			}
		}
		fields = append(fields, schemaField{
			Path: path, Type: typ, Format: format, Description: description,
		})
		_ = required[name]
	}
	return fields
}
