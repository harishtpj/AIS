package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/harishtpj/ais/internal/ontology"
	"github.com/harishtpj/ais/internal/service"
	"github.com/harishtpj/ais/internal/slm"
)

type Manager struct {
	mutex    sync.RWMutex
	entities []ontology.Entity
	plans    map[string]Plan
	client   *http.Client
	slm      *slm.Client
}

func NewManager(document *ontology.Document, slmClient *slm.Client) (*Manager, error) {
	if document == nil {
		return nil, fmt.Errorf("canonical ontology document is required")
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &Manager{
		entities: append([]ontology.Entity(nil), document.Entities...),
		plans:    make(map[string]Plan),
		client:   &http.Client{},
		slm:      slmClient,
	}, nil
}

func (m *Manager) Refresh(ctx context.Context, services map[string]*service.Service) error {
	next := make(map[string]Plan)
	var refreshErr error
	for name, svc := range services {
		if svc == nil || svc.URL == nil {
			continue
		}
		planSet, err := m.loadServicePlans(ctx, name, svc)
		if err != nil {
			refreshErr = fmt.Errorf("refresh translation plans for %q: %w", name, err)
			continue
		}
		for key, plan := range planSet {
			next[key] = plan
		}
	}
	m.mutex.Lock()
	m.plans = next
	m.mutex.Unlock()
	return refreshErr
}

func (m *Manager) loadServicePlans(ctx context.Context, name string, svc *service.Service) (map[string]Plan, error) {
	target := svc.TargetURL()
	if target == nil {
		return nil, fmt.Errorf("service %q has no endpoint", name)
	}
	openAPIURL := *target
	openAPIURL.Path = strings.TrimRight(target.Path, "/") + "/openapi.json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, openAPIURL.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := m.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		// OpenAPI discovery is optional; services such as WireMock may not
		// expose an OpenAPI document.
		return nil, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAPI endpoint returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	operations, err := parseOpenAPI(raw)
	if err != nil {
		return nil, err
	}
	plans := make(map[string]Plan)
	for _, operation := range operations {
		plan, ok, err := m.buildPlan(name, operation)
		if err != nil {
			return nil, err
		}
		if ok {
			plans[planKey(name, operation.Method, operation.Path)] = plan
		}
	}
	return plans, nil
}

func (m *Manager) buildPlan(serviceName string, operation backendOperation) (Plan, bool, error) {
	var best ontology.Entity
	bestScore := 0
	for _, entity := range m.entities {
		score := scoreEntity(entity, operation.Request, operation.Response)
		if score > bestScore {
			best, bestScore = entity, score
		}
	}
	if bestScore == 0 {
		return Plan{}, false, nil
	}
	requestMappings := inferMappings(best, operation.Request)
	responseFields := operation.Response
	if len(responseFields) == 0 {
		// Some OpenAPI generators emit an empty response schema even when the
		// response uses the same object shape as the request.
		responseFields = operation.Request
	}
	responseMappings := inferMappingsFromBackend(best, responseFields)
	if m.slm != nil {
		candidates := unresolvedCandidates(best, operation.Request, requestMappings)
		if len(candidates) > 0 {
			if suggestions, err := m.slm.SuggestMappings(context.Background(), candidates); err == nil {
				requestMappings = mergeSuggestions(requestMappings, suggestions, true)
			}
		}
	}
	if len(requestMappings) == 0 && len(responseMappings) == 0 {
		return Plan{}, false, nil
	}
	return Plan{
		Service: serviceName, BackendPath: operation.Path, Method: operation.Method,
		Request: requestMappings, Response: responseMappings,
	}, true, nil
}

func (m *Manager) Translate(serviceName, method, path string, body []byte, response bool) ([]byte, bool, error) {
	key := planKey(serviceName, method, path)
	m.mutex.RLock()
	plan, ok := m.plans[key]
	m.mutex.RUnlock()
	if !ok || len(body) == 0 {
		return body, false, nil
	}
	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, false, fmt.Errorf("decode JSON for translation: %w", err)
	}
	var err error
	mappings := plan.Request
	if response {
		mappings = plan.Response
	}
	if len(mappings) == 0 {
		return body, false, nil
	}
	var output any
	switch value := input.(type) {
	case map[string]any:
		translated, applyErr := NewEngine().Apply(value, mappings)
		output = translated
		err = applyErr
		if err != nil {
			return nil, false, err
		}
		if response {
			preserveUnmappedFields(value, translated, mappings)
		}
	case []any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			object, ok := item.(map[string]any)
			if !ok {
				return body, false, nil
			}
			translated, applyErr := NewEngine().Apply(object, mappings)
			if applyErr != nil {
				return nil, false, applyErr
			}
			items = append(items, translated)
		}
		output = items
	default:
		return body, false, nil
	}
	translated, err := json.Marshal(output)
	if err != nil {
		return nil, false, err
	}
	return translated, true, nil
}

func preserveUnmappedFields(input, output map[string]any, mappings []Mapping) {
	for key, value := range input {
		mapped := false
		for _, mapping := range mappings {
			if mapping.From == key || strings.HasPrefix(mapping.From, key+".") {
				mapped = true
				break
			}
		}
		if !mapped {
			output[key] = value
		}
	}
}

func planKey(serviceName, method, path string) string {
	return serviceName + "\x00" + strings.ToUpper(method) + "\x00" + path
}

func scoreEntity(entity ontology.Entity, request, response []schemaField) int {
	score := 0
	for name := range entity.Fields {
		for _, field := range append(append([]schemaField{}, request...), response...) {
			score += fieldScore(name, field)
		}
	}
	return score
}

func inferMappings(entity ontology.Entity, fields []schemaField) []Mapping {
	var mappings []Mapping
	for name := range entity.Fields {
		if field, ok := bestField(name, fields); ok {
			mappings = append(mappings, Mapping{From: name, To: field.Path})
		}
	}
	return mappings
}

func inferMappingsFromBackend(entity ontology.Entity, fields []schemaField) []Mapping {
	var mappings []Mapping
	for name := range entity.Fields {
		if field, ok := bestField(name, fields); ok {
			mappings = append(mappings, Mapping{From: field.Path, To: name})
		}
	}
	return mappings
}

func bestField(name string, fields []schemaField) (schemaField, bool) {
	best := schemaField{}
	bestScore := 0
	for _, field := range fields {
		score := fieldScore(name, field)
		if score > bestScore {
			best, bestScore = field, score
		}
	}
	return best, bestScore > 0
}

func fieldScore(canonical string, backend schemaField) int {
	canonical = lastPathPart(canonical)
	backendName := lastPathPart(backend.Path)
	if normalize(canonical) == normalize(backendName) {
		return 100
	}
	canonicalTokens := tokens(canonical)
	backendTokens := tokens(backendName)
	overlap := 0
	for _, left := range canonicalTokens {
		for _, right := range backendTokens {
			if left == right {
				overlap++
			}
		}
	}
	if overlap == 0 {
		return 0
	}
	score := overlap * 20
	if backend.Type != "" && canonical != "" {
		score += 5
	}
	return score
}

func unresolvedCandidates(entity ontology.Entity, fields []schemaField, mappings []Mapping) []slm.Candidate {
	mapped := make(map[string]bool)
	for _, mapping := range mappings {
		mapped[mapping.From] = true
	}
	var candidates []slm.Candidate
	for name, field := range entity.Fields {
		if mapped[name] {
			continue
		}
		for _, backend := range fields {
			candidates = append(candidates, slm.Candidate{
				CanonicalPath: name, CanonicalType: field.Type, CanonicalFormat: field.Format,
				CanonicalDescription: field.Description, BackendPath: backend.Path,
				BackendType: backend.Type, BackendFormat: backend.Format,
				BackendDescription: backend.Description,
			})
		}
	}
	return candidates
}

func mergeSuggestions(mappings []Mapping, suggestions []slm.Suggestion, request bool) []Mapping {
	for _, suggestion := range suggestions {
		if !suggestion.Match || suggestion.Confidence < 0.75 {
			continue
		}
		if request {
			mappings = append(mappings, Mapping{From: suggestion.CanonicalPath, To: suggestion.BackendPath})
		} else {
			mappings = append(mappings, Mapping{From: suggestion.BackendPath, To: suggestion.CanonicalPath})
		}
	}
	return mappings
}

func normalize(value string) string {
	value = strings.ToLower(value)
	var b bytes.Buffer
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			b.WriteRune(char)
		}
	}
	return b.String()
}

func tokens(value string) []string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	var words []string
	var current strings.Builder
	for _, char := range value {
		if char >= 'A' && char <= 'Z' {
			if current.Len() > 0 {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
			current.WriteRune(char + ('a' - 'A'))
			continue
		}
		if char == ' ' {
			if current.Len() > 0 {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
			continue
		}
		current.WriteRune(char)
	}
	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
	}
	return words
}

func lastPathPart(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}
