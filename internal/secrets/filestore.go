package secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// credentialsFile is the on-disk shape of FileStore's storage. It is kept
// intentionally separate from config.Settings so the file holding secrets
// never gets mixed up with SetFree's shareable, inspectable configuration.
type credentialsFile struct {
	Gateways map[string]credentialEntry `toml:"gateways,omitempty"`
}

type credentialEntry struct {
	APIKey string `toml:"api_key"`
}

// FileStore is the default Store: API keys live in credentials.toml inside
// SetFree's config directory, created with 0600 permissions on Unix so only
// the owning user can read it.
type FileStore struct {
	path string
}

// NewFileStore returns a FileStore backed by credentials.toml in dir.
func NewFileStore(dir string) *FileStore {
	return &FileStore{path: filepath.Join(dir, "credentials.toml")}
}

func (f *FileStore) load() (*credentialsFile, error) {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return &credentialsFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c credentialsFile
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", f.path, err)
	}
	return &c, nil
}

func (f *FileStore) save(c *credentialsFile) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	dir := filepath.Dir(f.path)
	tmp, err := os.CreateTemp(dir, ".credentials-*.toml.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}

func (f *FileStore) Get(gateway string) (string, bool, error) {
	c, err := f.load()
	if err != nil {
		return "", false, err
	}
	entry, ok := c.Gateways[gateway]
	if !ok || entry.APIKey == "" {
		return "", false, nil
	}
	return entry.APIKey, true, nil
}

func (f *FileStore) Set(gateway, key string) error {
	c, err := f.load()
	if err != nil {
		return err
	}
	if c.Gateways == nil {
		c.Gateways = map[string]credentialEntry{}
	}
	c.Gateways[gateway] = credentialEntry{APIKey: key}
	return f.save(c)
}

func (f *FileStore) Delete(gateway string) error {
	c, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := c.Gateways[gateway]; !ok {
		return nil
	}
	delete(c.Gateways, gateway)
	return f.save(c)
}

func (f *FileStore) Reset() error {
	err := os.Remove(f.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
