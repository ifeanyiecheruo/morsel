package local

import (
	"context"
	"io"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localBlobStore struct{}

func (b *localBlobStore) Get(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return nil, platform.ErrNotImplemented
}
func (b *localBlobStore) Put(_ context.Context, _, _ string, _ io.Reader, _ int64) error {
	return platform.ErrNotImplemented
}
func (b *localBlobStore) List(_ context.Context, _, _, _ string, _ int) ([]string, string, error) {
	return nil, "", platform.ErrNotImplemented
}
func (b *localBlobStore) Delete(_ context.Context, _, _ string) error {
	return platform.ErrNotImplemented
}
func (b *localBlobStore) Usage(_ context.Context, _ string) (int64, error) {
	return 0, platform.ErrNotImplemented
}
