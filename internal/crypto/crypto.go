package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

var (
	ulidMu      sync.Mutex
	ulidEntropy = ulid.Monotonic(rand.Reader, 0)
)

// NewULID generates a lexicographically sortable unique identifier.
func NewULID() string {
	ulidMu.Lock()
	defer ulidMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulidEntropy).String()
}

// Cipher provides AES-256-GCM encryption for database-persisted secrets.
type Cipher struct {
	key []byte
}

// NewCipher constructs a Cipher using a 32-byte master key.
func NewCipher(masterKey []byte) (*Cipher, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("expected 32-byte key, got %d", len(masterKey))
	}
	cp := make([]byte, 32)
	copy(cp, masterKey)
	return &Cipher{key: cp}, nil
}

// DeriveSubKey returns a deterministic 32-byte subkey for a specific purpose (e.g. JWT signing).
func (c *Cipher) DeriveSubKey(purpose string) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

// EncryptString encrypts plaintext and returns a base64-encoded ciphertext.
// An empty string returns an empty string.
func (c *Cipher) EncryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	enc, err := encryptAESGCM(c.key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// DecryptString decrypts a base64-encoded ciphertext produced by EncryptString.
// An empty string returns an empty string.
func (c *Cipher) DecryptString(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64 ciphertext: %w", err)
	}
	plain, err := decryptAESGCM(c.key, raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

var bundleMagic = []byte{'M', 'S', 'B', '1'}

// EncryptWithPassphrase encrypts arbitrary bytes using Argon2id key derivation + AES-256-GCM.
func EncryptWithPassphrase(plaintext []byte, passphrase string) ([]byte, error) {
	if passphrase != "" && len(passphrase) < 6 {
		return nil, errors.New("passphrase must be at least 6 characters (or empty)")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(passphrase), salt, 2, 64*1024, 4, 32)
	enc, err := encryptAESGCM(key, plaintext)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(bundleMagic)+len(salt)+len(enc))
	out = append(out, bundleMagic...)
	out = append(out, salt...)
	out = append(out, enc...)
	return out, nil
}

// DecryptWithPassphrase decrypts a payload produced by EncryptWithPassphrase.
func DecryptWithPassphrase(blob []byte, passphrase string) ([]byte, error) {
	headerLen := len(bundleMagic) + 16
	if len(blob) <= headerLen {
		return nil, errors.New("invalid migration bundle format")
	}
	if !bytes.Equal(blob[:len(bundleMagic)], bundleMagic) {
		return nil, errors.New("unsupported migration bundle header")
	}
	salt := blob[len(bundleMagic):headerLen]
	ciphertext := blob[headerLen:]
	key := argon2.IDKey([]byte(passphrase), salt, 2, 64*1024, 4, 32)
	plain, err := decryptAESGCM(key, ciphertext)
	if err != nil {
		return nil, errors.New("invalid passphrase or corrupted migration bundle")
	}
	return plain, nil
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func decryptAESGCM(key, payload []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// GenerateAPIKey creates a new client auth_key, its display prefix, and SHA-256 hash.
func GenerateAPIKey() (plaintext, prefix, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = io.ReadFull(rand.Reader, buf); err != nil {
		return "", "", "", err
	}
	plaintext = "ms_" + hex.EncodeToString(buf)
	prefix = plaintext[:11]
	hash = HashAPIKey(plaintext)
	return plaintext, prefix, hash, nil
}

// HashAPIKey computes the hex-encoded SHA-256 hash of an auth_key.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// HashPassword hashes an admin password with bcrypt.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword compares a bcrypt hash against a plaintext password.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// RandomToken returns a URL-safe random hex token of n bytes.
func RandomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
