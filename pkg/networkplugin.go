package pkg

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPlugin interface {
	Create(string, string) error
	Delete(string, string) error
}

type NetworkPluginDedploy struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (n *NetworkPluginDedploy) Create(string, string) error {
	panic("implement me")
}

func (n *NetworkPluginDedploy) Delete(string, string) error {
	panic("implement me")
}


