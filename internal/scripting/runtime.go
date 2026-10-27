package scripting

/*
#cgo CFLAGS: -I${SRCDIR}/../../vendors/gruby/mruby-build/mruby/build/host/include
#cgo LDFLAGS: -L${SRCDIR}/../../vendors/gruby/mruby-build/mruby/build/host/lib -lmruby -lm
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/zhulik/gruby"
)

type Runtime struct {
	vm          *gruby.GRuby
	registry    *service.Registry
	router      *router.Router
	Addr        string
	UnifiedAuth bool
}

func NewRuntime(registry *service.Registry, router *router.Router) (*Runtime, error) {
	vm, err := gruby.New()
	if err != nil {
		return nil, fmt.Errorf("initialize mruby: %w", err)
	}

	rt := &Runtime{vm: vm, registry: registry, router: router, Addr: "localhost:8080"}
	rt.registerBindings()

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
