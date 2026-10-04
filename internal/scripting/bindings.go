package scripting

import (
	"github.com/harishtpj/ais/internal/service"
	"github.com/zhulik/gruby"
)

func (r *Runtime) registerBindings() {
	ais := r.vm.DefineClass("AIS", nil)
	ais.DefineClassMethod("configure", r.configure, gruby.ArgsBlock())
	ais.DefineMethod("service", r.addService, gruby.ArgsReq(2))
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

	svc, err := service.New(name, url)
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
