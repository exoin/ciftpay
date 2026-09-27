package crypto

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"
)

type mockSecretProvider struct {
	key []byte
	err error
}

func (m *mockSecretProvider) FetchMasterKey(ctx context.Context) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return m.key, m.err
}

func TestEnvSecretProvider_ValidKey(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	t.Setenv("TEST_MASTER_KEY_B64", raw)

	p := NewEnvSecretProvider("TEST_MASTER_KEY_B64")
	key, err := p.FetchMasterKey(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("got key length %d, want 32", len(key))
	}
}

func TestEnvSecretProvider_DefaultFallback(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	p := &EnvSecretProvider{
		EnvVar:        "NON_EXISTENT_VAR_12345",
		DefaultKeyB64: raw,
	}

	key, err := p.FetchMasterKey(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("got key length %d, want 32", len(key))
	}
}

func TestEnvSecretProvider_MissingEnvVar(t *testing.T) {
	_ = os.Unsetenv("TEST_MISSING_VAR")
	p := NewEnvSecretProvider("TEST_MISSING_VAR")
	_, err := p.FetchMasterKey(context.Background())
	if err == nil {
		t.Fatal("expected error for missing environment variable")
	}
}

func TestEnvSecretProvider_InvalidBase64(t *testing.T) {
	t.Setenv("TEST_BAD_B64", "not-valid-base64!!!")
	p := NewEnvSecretProvider("TEST_BAD_B64")
	_, err := p.FetchMasterKey(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestEnvSecretProvider_InvalidLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("short-key"))
	t.Setenv("TEST_SHORT_KEY", short)
	p := NewEnvSecretProvider("TEST_SHORT_KEY")
	_, err := p.FetchMasterKey(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestNewKeyring_MockProvider(t *testing.T) {
	mockKey := []byte("01234567890123456789012345678901")
	mock := &mockSecretProvider{key: mockKey}

	kr, err := NewKeyring(context.Background(), mock, "test-pepper")
	if err != nil {
		t.Fatalf("unexpected NewKeyring error: %v", err)
	}

	plaintext := "test personal data"
	ciphertext, err := kr.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}

	decrypted, err := kr.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestNewKeyring_NilProvider(t *testing.T) {
	_, err := NewKeyring(context.Background(), nil, "pepper")
	if err == nil {
		t.Fatal("expected error when provider is nil")
	}
}

func TestNewKeyring_ProviderError(t *testing.T) {
	mock := &mockSecretProvider{err: errors.New("vault unreachable")}
	_, err := NewKeyring(context.Background(), mock, "pepper")
	if err == nil {
		t.Fatal("expected error when provider returns error")
	}
}

func TestNewKeyring_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewEnvSecretProvider("ANY_VAR")
	_, err := NewKeyring(ctx, p, "pepper")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
