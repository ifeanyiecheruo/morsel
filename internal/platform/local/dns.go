// *.morsel.localhost resolves natively via RFC 6761 — all DNS methods are no-ops.
package local

import "context"

type localDNSProvider struct{}

func (ld *localDNSProvider) CreateRecord(_ context.Context, _, _, _, _ string, _ int) error {
	return nil
}
func (ld *localDNSProvider) DeleteRecord(_ context.Context, _, _, _ string) error { return nil }
func (ld *localDNSProvider) RecordExists(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
