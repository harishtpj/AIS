package scripting

/*
#cgo CFLAGS: -I${SRCDIR}/../../vendors/gruby/mruby-build/mruby/build/host/include
#cgo LDFLAGS: -L${SRCDIR}/../../vendors/gruby/mruby-build/mruby/build/host/lib -lmruby -lm
*/
import "C"

import (
	"embed"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/zhulik/gruby"
)

type Runtime struct {
	vm          *gruby.GRuby
	vmMu        sync.Mutex
	policyMu    sync.RWMutex
	policies    []Policy
	registry    *service.Registry
	router      *router.Router
	Addr        string
	UnifiedAuth bool
}

//go:embed ais
var supportFiles embed.FS

func NewRuntime(registry *service.Registry, router *router.Router) (*Runtime, error) {
	vm, err := gruby.New()
	if err != nil {
		return nil, fmt.Errorf("initialize mruby: %w", err)
	}

	rt := &Runtime{vm: vm, registry: registry, router: router, Addr: "localhost:8080"}
	rt.registerBindings()

	for _, name := range []string{"ais/time.rb", "ais/policy.rb", "ais/datetime.rb"} {
		source, readErr := supportFiles.ReadFile(name)
		if readErr != nil {
			vm.Close()
			return nil, fmt.Errorf("read embedded scripting support %q: %w", name, readErr)
		}
		if _, evalErr := vm.LoadString(string(source)); evalErr != nil {
			vm.Close()
			return nil, fmt.Errorf("load embedded scripting support %q: %w", name, evalErr)
		}
	}

	return rt, nil
}

func (r *Runtime) Close() {
	if r.vm != nil {
		r.vm.Close()
		r.vm = nil
	}
}

func (r *Runtime) LoadFile(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Ruby config: %w", err)
	}

	_, _, err = r.vm.LoadFile(path, string(source))
	if err != nil {
		return fmt.Errorf("execute Ruby config: %w", err)
	}

	return nil
}

func (r *Runtime) Eval(source string) error {
	_, err := r.vm.LoadString(source)
	if err != nil {
		return fmt.Errorf("execute Ruby: %w", err)
	}

	return nil
}

var _ = unsafe.Pointer(nil)
