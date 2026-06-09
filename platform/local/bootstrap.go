package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localBootstrapper struct{}

func (b *localBootstrapper) Prompts() []platform.Prompt                              { return nil }
func (b *localBootstrapper) Plan(_ map[string]string) platform.Plan                  { return platform.Plan{} }
func (b *localBootstrapper) Provision(_ context.Context, _ map[string]string) error {
	return platform.ErrNotImplemented
}
