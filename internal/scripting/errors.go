package scripting

import (
	"github.com/zhulik/gruby"
)

func rubyError(vm *gruby.GRuby, class, message string) (gruby.Value, gruby.Value) {
	excClass := vm.Class(class, nil)

	messageValue := gruby.MustToRuby(vm, message)

	exc, err := excClass.New(messageValue)
	if err != nil {
		panic("failed to create Ruby exception: " + err.Error())
	}

	return vm.NilValue(), exc
}
