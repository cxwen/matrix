package pkg

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Matrix interface {
	Create(string, string) error
	Delete(string, string) error
}

type MatrixDedploy struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (m *MatrixDedploy) Create(string, string) error {
	panic("implement me")
}

func (m *MatrixDedploy) Delete(string, string) error {
	panic("implement me")
}


