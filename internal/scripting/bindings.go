package scripting

import (
	"strings"

	"github.com/harishtpj/ais/internal/service"
	"github.com/zhulik/gruby"
)

func (r *Runtime) registerBindings() {
	ais := r.vm.DefineClass("AIS", nil)
	ais.DefineClassMethod("configure", r.configure, gruby.ArgsBlock())
	ais.DefineMethod("service", r.addService, gruby.ArgsArg(2, 1))
	ais.DefineMethod("unified_auth", r.enableUnifiedAuth, gruby.ArgsReq(1))
	ais.DefineMethod("route", r.addRoute, gruby.ArgsReq(2))
	ais.DefineMethod("bind", r.bind, gruby.ArgsReq(1))
}

func (r *Runtime) configure(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()

	if len(args) != 1 {
		return rubyError(vm, "ArgumentError", "configure expects a single argument")
	}

	if args[0].Type() != gruby.TypeProc {
		return rubyError(vm, "ArgumentError", "configure expects a block")
	}

	// rb: config = AIS.new
	config, err := self.Call("new")
	if err != nil {
		return rubyError(vm, "RuntimeError", "failed to create AIS configuration: "+err.Error())
	}

	// rb: config.instance_exec(&block)
	_, err = config.CallBlock("instance_exec", args[0])
	if err != nil {
		return rubyError(vm, "RuntimeError", "failed to execute configure block: "+err.Error())
	}

	return config, nil
}

func (r *Runtime) addService(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()

	if args[0].Type() != gruby.TypeSymbol {
		return rubyError(vm, "ArgumentError", "first argument must be a symbol")
	}
	name := args[0].String()
	url, err := gruby.ToGo[string](args[1])
	if err != nil {
		return rubyError(vm, "ArgumentError", "second argument must be a string")
	}

	authStrategy := service.AuthNone
	if len(args) >= 3 && args[2].Type() != gruby.TypeNil {
		if args[2].Type() != gruby.TypeHash {
			return rubyError(vm, "ArgumentError", "third argument must be an options hash")
		}
		options, err := gruby.ToGo[gruby.Hash](args[2])
		if err != nil {
			return rubyError(vm, "ArgumentError", "could not read service options")
		}
		for _, key := range options.Keys() {
			if strings.TrimPrefix(key.String(), ":") != "auth" {
				continue
			}
			value := options.Get(key)
			if value.Type() != gruby.TypeSymbol && value.Type() != gruby.TypeString {
				return rubyError(vm, "ArgumentError", "auth must be :sessions or :opq_tok")
			}
			authStrategy = strings.TrimPrefix(value.String(), ":")
		}
	}

	svc, err := service.New(name, url, authStrategy)
	if err != nil {
		return rubyError(vm, "RuntimeError", "failed to create service: "+err.Error())
	}

	if err := r.registry.Register(svc); err != nil {
		return rubyError(vm, "RuntimeError", "failed to register service: "+err.Error())
	}

	return vm.NilValue(), nil
}

func (r *Runtime) addRoute(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()

	path, err := gruby.ToGo[string](args[0])
	if err != nil {
		return rubyError(vm, "ArgumentError", "first argument must be a string")
	}
	if args[1].Type() != gruby.TypeSymbol {
		return rubyError(vm, "ArgumentError", "second argument must be a symbol")
	}
	serviceName := args[1].String()

	if !r.registry.Exists(serviceName) {
		return rubyError(vm, "ArgumentError", "service not found: "+serviceName)
	}

	if err := r.router.AddRoute(path, serviceName); err != nil {
		return rubyError(vm, "RuntimeError", "failed to add route: "+err.Error())
	}

	return vm.NilValue(), nil
}

func (r *Runtime) bind(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()
	addr, err := gruby.ToGo[string](args[0])
	if err != nil {
		return rubyError(vm, "ArgumentError", "first argument must be a string")
	}

	r.Addr = addr
	return vm.NilValue(), nil
}

func (r *Runtime) enableUnifiedAuth(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
	args := vm.GetArgs()

	switch args[0].Type() {
	case gruby.TypeTrue:
		r.UnifiedAuth = true
	case gruby.TypeFalse:
		r.UnifiedAuth = false
	default:
		return rubyError(vm, "ArgumentError", "unified_auth expects true or false")
	}

	return vm.NilValue(), nil
}
