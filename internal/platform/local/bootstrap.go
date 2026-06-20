package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localBootstrapper struct{}

func (lb *localBootstrapper) Prompts() []platform.Prompt             { return nil }
func (lb *localBootstrapper) Plan(_ map[string]string) platform.Plan { return platform.Plan{} }
func (lb *localBootstrapper) Provision(_ context.Context, _ map[string]string) error {
	return platform.ErrNotImplemented
}
