package crypto

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// SecretProvider abstracts fetching sensitive secrets (such as the envelope encryption
// master key) from backends such as local environment variables, AWS Secrets Manager, or Vault.
type SecretProvider interface {
	FetchMasterKey(ctx context.Context) ([]byte, error)
}

// EnvSecretProvider implements SecretProvider by reading from an environment variable.
type EnvSecretProvider struct {
	EnvVar        string
	DefaultKeyB64 string
}

// NewEnvSecretProvider constructs an EnvSecretProvider that reads from envVar.
// If envVar is omitted or empty, it defaults to "MASTER_KEY_B64".
func NewEnvSecretProvider(envVar ...string) *EnvSecretProvider {
	v := "MASTER_KEY_B64"
	if len(envVar) > 0 && envVar[0] != "" {
		v = envVar[0]
	}
	return &EnvSecretProvider{EnvVar: v}
}

// FetchMasterKey reads and decodes the base64 master key from the environment.
func (p *EnvSecretProvider) FetchMasterKey(ctx context.Context) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	envVar := p.EnvVar
	if envVar == "" {
		envVar = "MASTER_KEY_B64"
	}
	val := os.Getenv(envVar)
	if val == "" {
		val = p.DefaultKeyB64
	}
	if val == "" {
		return nil, fmt.Errorf("crypto: environment variable %q is not set", envVar)
	}
	key, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("crypto: %s is not valid base64: %w", envVar, err)
	}
	if len(key) != masterKeyLen {
		return nil, fmt.Errorf("crypto: %s must decode to %d bytes, got %d", envVar, masterKeyLen, len(key))
	}
	return key, nil
}

// NewKeyring builds a Keyring using the provided SecretProvider.
func NewKeyring(ctx context.Context, sp SecretProvider, pepper string) (*Keyring, error) {
	if sp == nil {
		return nil, errors.New("crypto: secret provider must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	masterKey, err := sp.FetchMasterKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to fetch master key: %w", err)
	}
	return New(masterKey, pepper)
}
