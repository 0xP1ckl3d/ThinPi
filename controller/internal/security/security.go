package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

func RandomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 1024 {
		return "", errors.New("password must be between 8 and 1024 characters")
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	if memory > 256*1024 || iterations > 10 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}

type Vault struct{ aead cipher.AEAD }

func NewVault(key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, errors.New("master key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Prefix a format version so later key/algorithm migrations can be explicit.
	out := []byte{1}
	out = append(out, nonce...)
	return v.aead.Seal(out, nonce, plaintext, []byte("thinpi-credential-v1")), nil
}

func (v *Vault) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 1+v.aead.NonceSize() || ciphertext[0] != 1 {
		return nil, errors.New("invalid encrypted credential")
	}
	nonce := ciphertext[1 : 1+v.aead.NonceSize()]
	return v.aead.Open(nil, nonce, ciphertext[1+v.aead.NonceSize():], []byte("thinpi-credential-v1"))
}

func ParseMasterKey(raw []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return raw, nil
	}
	return nil, errors.New("master key must be 32 raw bytes or base64 encoding of 32 bytes")
}

func DeterministicDevKey() []byte {
	sum := sha256.Sum256([]byte("ThinPi development key; never use in production"))
	return sum[:]
}

func Uint64Token() (uint64, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	return binary.BigEndian.Uint64(b[:]), err
}
