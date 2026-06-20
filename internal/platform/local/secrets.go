package local

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// localSecretStore persists secrets as a JSON map at ~/.morsel/local/secrets.json.
// Values are base64-encoded to handle arbitrary byte payloads.
type localSecretStore struct {
	mu   sync.Mutex
	path string
}

func newLocalSecretStore() *localSecretStore {
	home, _ := os.UserHomeDir()
	return &localSecretStore{
		path: filepath.Join(home, ".morsel", "local", "secrets.json"),
	}
}

func (ls *localSecretStore) Get(_ context.Context, name string) ([]byte, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	data, err := ls.load()
	if err != nil {
		return nil, err
	}
	encoded, ok := data[name]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return base64.StdEncoding.DecodeString(encoded)
}

func (ls *localSecretStore) Set(_ context.Context, name string, value []byte) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	data, err := ls.load()
	if err != nil && !errors.Is(err, platform.ErrSecretNotFound) {
		return err
	}
	if data == nil {
		data = make(map[string]string)
	}
	data[name] = base64.StdEncoding.EncodeToString(value)
	return ls.save(data)
}

func (ls *localSecretStore) Delete(_ context.Context, name string) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	data, err := ls.load()
	if err != nil {
		return err
	}
	delete(data, name)
	return ls.save(data)
}

func (ls *localSecretStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(ls.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (ls *localSecretStore) save(data map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(ls.path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ls.path, raw, 0600)
}
