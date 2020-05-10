package pkg

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Dns interface {
	Create(string, int, string, string) error
	Delete(string, string) error
}

type DnsDedploy struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (d *DnsDedploy) Create(string, int, string, string) error {
	panic("implement me")
}

func (d *DnsDedploy) Delete(string, string) error {
	panic("implement me")
}


