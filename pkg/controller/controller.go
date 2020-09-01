package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, util.BuildVariables) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, bv util.BuildVariables) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, bv); err != nil {
			return err
		}
	}
	return nil
}
