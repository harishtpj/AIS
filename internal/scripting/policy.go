package scripting

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/harishtpj/ais/internal/geolocation"
	"github.com/zhulik/gruby"
)

// Policy stores Go-native matching metadata and a retained Ruby Proc. Matching
// is performed in Go; only matching callbacks enter the mruby VM.
type Policy struct {
	Name         string
	Services     []string
	Routes       []string
	UsesLocation bool
	builder      gruby.Value
	evaluator    gruby.Value
}

func (r *Runtime) addPolicy(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()
	if len(args) != 2 {
		return rubyError(vm, "ArgumentError", "policy expects a name and a block")
	}
	name, err := gruby.ToGo[string](args[0])
	if err != nil || strings.TrimSpace(name) == "" {
		return rubyError(vm, "ArgumentError", "policy name must be a non-empty string")
	}
	if args[1].Type() != gruby.TypeProc {
		return rubyError(vm, "ArgumentError", "policy expects a block")
	}

	builderClass := vm.Class("AISPolicyBuilder", nil)
	builder, err := builderClass.New(gruby.MustToRuby(vm, name))
	if err != nil {
		return rubyError(vm, "RuntimeError", "create policy builder: "+err.Error())
	}
	if rootable, ok := builder.(interface{ GCProtect() }); ok {
		rootable.GCProtect()
	}
	if _, err := builder.CallBlock("instance_exec", args[1]); err != nil {
		return rubyError(vm, "RuntimeError", "execute policy definition: "+err.Error())
	}

	servicesValue, err := builder.Call("services")
	if err != nil {
		return rubyError(vm, "RuntimeError", "read policy services: "+err.Error())
	}
	var services []string
	if err := gruby.Decode(&services, servicesValue); err != nil {
		return rubyError(vm, "ArgumentError", "for_services must contain service names")
	}

	routesValue, err := builder.Call("routes")
	if err != nil {
		return rubyError(vm, "RuntimeError", "read policy routes: "+err.Error())
	}
	var routes []string
	if err := gruby.Decode(&routes, routesValue); err != nil {
		return rubyError(vm, "ArgumentError", "for_routes must contain route patterns")
	}

	evaluator, err := builder.Call("evaluator")
	if err != nil || evaluator.Type() != gruby.TypeProc {
		return rubyError(vm, "ArgumentError", "policy must define evaluate do ... end")
	}
	if rootable, ok := evaluator.(interface{ GCProtect() }); ok {
		rootable.GCProtect()
	}
	usesLocationValue, err := builder.Call("location_required?")
	if err != nil {
		return rubyError(vm, "RuntimeError", "read uses_location flag: "+err.Error())
	}
	usesLocation := usesLocationValue.Type() == gruby.TypeTrue

	policy := Policy{
		Name: name, Services: services, Routes: routes,
		UsesLocation: usesLocation, builder: builder, evaluator: evaluator,
	}
	r.policyMu.Lock()
	defer r.policyMu.Unlock()
	for _, existing := range r.policies {
		if existing.Name == name {
			return rubyError(vm, "ArgumentError", "duplicate policy name: "+name)
		}
	}
	r.policies = append(r.policies, policy)
	return vm.NilValue(), nil
}

// ValidatePolicies checks static policy metadata after the configuration file
// has loaded and before the HTTP listener starts.
func (r *Runtime) ValidatePolicies() error {
	r.policyMu.RLock()
	defer r.policyMu.RUnlock()
	for _, policy := range r.policies {
		if len(policy.Services) == 0 {
			return fmt.Errorf("policy %q must declare for_services", policy.Name)
		}
		if len(policy.Routes) == 0 {
			return fmt.Errorf("policy %q must declare for_routes", policy.Name)
		}
		if policy.evaluator.Type() != gruby.TypeProc {
			return fmt.Errorf("policy %q has no evaluator", policy.Name)
		}
		for _, route := range policy.Routes {
			if err := validatePolicyPattern(route); err != nil {
				return fmt.Errorf("policy %q: %w", policy.Name, err)
			}
		}
		for _, name := range policy.Services {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("policy %q contains an empty service name", policy.Name)
			}
		}
	}
	return nil
}

// PoliciesNeedLocation lets the gateway avoid a geolocation lookup when no
// applicable policy has declared that it needs location metadata.
func (r *Runtime) PoliciesNeedLocation(path, baseService string) bool {
	for _, policy := range r.matchingPolicies(path, baseService) {
		if policy.UsesLocation {
			return true
		}
	}
	return false
}

// EvaluatePolicies executes matching callbacks in declaration order. A policy
// can only route to a service included in its for_services allow-list. The
// first matching policy that calls route_to wins.
func (r *Runtime) EvaluatePolicies(
	method, path, clientIP, baseService string,
	location geolocation.Location,
	now time.Time,
) (serviceName string, applied bool, err error) {
	policies := r.matchingPolicies(path, baseService)
	if len(policies) == 0 {
		return "", false, nil
	}

	// mruby state and all values owned by it are accessed under one lock.
	r.vmMu.Lock()
	defer r.vmMu.Unlock()

	locValue, err := r.makeLocationValue(location)
	if err != nil {
		return "", false, fmt.Errorf("build policy location context: %w", err)
	}
	requestValue, err := r.vm.Class("AISPolicyRequest", nil).New(
		gruby.MustToRuby(r.vm, method),
		gruby.MustToRuby(r.vm, path),
		gruby.MustToRuby(r.vm, clientIP),
		locValue,
		gruby.MustToRuby(r.vm, int(now.Unix())),
	)
	if err != nil {
		return "", false, fmt.Errorf("build Ruby request context: %w", err)
	}

	for _, policy := range policies {
		if _, err := policy.builder.Call("__begin_request", requestValue); err != nil {
			return "", false, fmt.Errorf("prepare policy %q: %w", policy.Name, err)
		}
		if _, err := policy.evaluator.Call("call", requestValue); err != nil {
			return "", false, fmt.Errorf("evaluate policy %q: %w", policy.Name, err)
		}
		decision, err := policy.builder.Call("__decision")
		if err != nil {
			return "", false, fmt.Errorf("read policy %q decision: %w", policy.Name, err)
		}
		if decision.Type() == gruby.TypeNil {
			continue
		}
		target, err := gruby.ToGo[string](decision)
		if err != nil || strings.TrimSpace(target) == "" {
			return "", false, fmt.Errorf("policy %q returned an invalid route_to decision", policy.Name)
		}
		if !slices.Contains(policy.Services, target) {
			return "", false, fmt.Errorf("policy %q attempted to route to undeclared service %q", policy.Name, target)
		}
		return target, true, nil
	}
	return "", false, nil
}

func (r *Runtime) matchingPolicies(path, baseService string) []Policy {
	r.policyMu.RLock()
	defer r.policyMu.RUnlock()
	matched := make([]Policy, 0, len(r.policies))
	for _, policy := range r.policies {
		if !slices.Contains(policy.Services, baseService) || !policyMatchesPath(policy.Routes, path) {
			continue
		}
		matched = append(matched, policy)
	}
	return matched
}

func (r *Runtime) makeLocationValue(location geolocation.Location) (gruby.Value, error) {
	latitude := r.vm.NilValue()
	if location.Latitude != nil {
		latitude = gruby.MustToRuby(r.vm, *location.Latitude)
	}
	longitude := r.vm.NilValue()
	if location.Longitude != nil {
		longitude = gruby.MustToRuby(r.vm, *location.Longitude)
	}
	return r.vm.Class("AISPolicyLocation", nil).New(
		gruby.MustToRuby(r.vm, location.Country),
		gruby.MustToRuby(r.vm, location.Region),
		gruby.MustToRuby(r.vm, location.RegionCode),
		gruby.MustToRuby(r.vm, location.City),
		latitude, longitude,
		gruby.MustToRuby(r.vm, location.Timezone),
		gruby.MustToRuby(r.vm, location.Source),
		gruby.MustToRuby(r.vm, location.Available),
	)
}

func policyMatchesPath(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == "/*" {
			return true
		}
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "*")
			base := strings.TrimSuffix(prefix, "/")
			if path == base || strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if path == pattern || strings.HasPrefix(path, pattern+"/") {
			return true
		}
	}
	return false
}

func validatePolicyPattern(pattern string) error {
	if pattern == "*" || pattern == "/*" {
		return nil
	}
	if !strings.HasPrefix(pattern, "/") || strings.ContainsAny(pattern, "?#") {
		return fmt.Errorf("route pattern %q must start with '/' and contain no query or fragment", pattern)
	}
	if strings.Count(pattern, "*") > 1 || (strings.Contains(pattern, "*") && !strings.HasSuffix(pattern, "*")) {
		return fmt.Errorf("route pattern %q only supports a single trailing '*' wildcard", pattern)
	}
	return nil
}
