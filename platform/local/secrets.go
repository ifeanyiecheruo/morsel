package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localSecretStore struct{}

func (s *localSecretStore) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, platform.ErrNotImplemented
}
func (s *localSecretStore) Set(_ context.Context, _ string, _ []byte) error {
	return platform.ErrNotImplemented
}
func (s *localSecretStore) Delete(_ context.Context, _ string) error {
	return platform.ErrNotImplemented
}
