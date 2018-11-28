package controller

import (
	"github.com/hawtio/hawtio-operator/pkg/controller/hawtio"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, hawtio.Add)
}
