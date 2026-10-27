package scripting

import (
	"strings"

	"github.com/harishtpj/ais/internal/service"
	"github.com/zhulik/gruby"
)

func (r *Runtime) registerBindings() {
	ais := r.vm.DefineClass("AIS", nil)
	ais.DefineClassMethod("configure", r.configure, gruby.ArgsBlock())
	ais.DefineMethod("service", r.addService, gruby.ArgsReq(2)|gruby.ArgsBlock())
	ais.DefineMethod("unified_auth", r.enableUnifiedAuth, gruby.ArgsReq(1))
	ais.DefineMethod("route", r.addRoute, gruby.ArgsReq(2))
	ais.DefineMethod("bind", r.bind, gruby.ArgsReq(1))

	hash := r.vm.Class("Hash", nil)
	hash.DefineMethod("auth", func(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
		args := vm.GetArgs()

		if len(args) != 1 {
			return rubyError(vm, "ArgumentError", "auth expects a single argument")
		}

		if args[0].Type() != gruby.TypeSymbol {
			return rubyError(vm, "ArgumentError", "auth expects a symbol")
		}

		// rb: self[:auth] = :kind
		val, err := self.Call("[]=", gruby.MustToRuby(vm, ":auth"), args[0])
		if err != nil {
			return rubyError(vm, "RuntimeError", err.Error())
		}

		return val, nil
	}, gruby.ArgsReq(1))
	hash.DefineMethod("auth_path", func(vm *gruby.GRuby, self gruby.Value) (gruby.Value, gruby.Value) {
		args := vm.GetArgs()

		if len(args) != 1 {
			return rubyError(vm, "ArgumentError", "auth_path expects a single argument")
		}

		if args[0].Type() != gruby.TypeString {
			return rubyError(vm, "ArgumentError", "auth_path expects a symbol")
		}

		// rb: self[:auth_path] = '/login_path'
		val, err := self.Call("[]=", gruby.MustToRuby(vm, ":auth_path"), args[0])
		if err != nil {
			return rubyError(vm, "RuntimeError", err.Error())
		}

		return val, nil
	}, gruby.ArgsReq(1))
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

	if len(args) < 2 || len(args) > 3 {
		return rubyError(vm, "ArgumentError", "service expects a name, URL, and optional block")
	}

	if args[0].Type() != gruby.TypeSymbol && args[0].Type() != gruby.TypeString {
		return rubyError(vm, "ArgumentError", "first argument must be a symbol or string")
	}
	name := strings.TrimPrefix(args[0].String(), ":")

	url, err := gruby.ToGo[string](args[1])
	if err != nil {
		return rubyError(vm, "ArgumentError", "second argument must be a URL string")
	}

	authStrategy := service.AuthNone
	authPath := "/login"

	if len(args) == 3 && args[2].Type() != gruby.TypeNil {
		if args[2].Type() != gruby.TypeProc {
			return rubyError(vm, "ArgumentError", "third argument must be a block")
		}

		hashClass := vm.Class("Hash", nil)
		options, err := hashClass.New()
		if err != nil {
			return rubyError(vm, "RuntimeError", "failed to create service options: "+err.Error())
		}

		_, err = options.CallBlock("instance_exec", args[2])
		if err != nil {
			return rubyError(vm, "RuntimeError", "failed to execute service options block: "+err.Error())
		}

		hash, err := gruby.ToGo[gruby.Hash](options)
		if err != nil {
			return rubyError(vm, "ArgumentError", "service options block must produce a hash")
		}

		for _, key := range hash.Keys() {
			switch strings.TrimPrefix(key.String(), ":") {
			case "auth":
				value := hash.Get(key)
				if value.Type() != gruby.TypeSymbol && value.Type() != gruby.TypeString {
					return rubyError(vm, "ArgumentError", "auth must be a symbol or string")
				}
				authStrategy = strings.TrimPrefix(value.String(), ":")

			case "auth_path":
				value, err := gruby.ToGo[string](hash.Get(key))
				if err != nil {
					return rubyError(vm, "ArgumentError", "auth_path must be a string")
				}
				authPath = value
			}
		}
	}

	svc, err := service.NewWithOptions(name, url, authStrategy, authPath)
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
