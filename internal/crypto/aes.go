package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"golang.org/x/crypto/bcrypt"
)

// AES implements the Crypto interface with real AES-256-GCM encryption
// and bcrypt-based admin-password verification.
//
// It is a drop-in replacement for Stub: NewAES constructs it from a
// human-supplied key string and a bcrypt hash of the management password,
// then it is assigned to Deps.Crypto in main.go.
type AES struct {
	key         []byte // 32-byte AES-256 key
	adminPwHash string // bcrypt hash of the management password
}

// Compile-time guarantee that AES satisfies Crypto.
var _ Crypto = (*AES)(nil)

// NewAES creates an AES crypto implementation.
//
//   - key is used as the AES-256 encryption key. If it is not exactly
//     32 bytes long it is hashed with SHA-256 to produce a 32-byte key.
//   - adminPwHash is a bcrypt hash used by VerifyAdminPw.
func NewAES(key string, adminPwHash string) (*AES, error) {
	var k []byte
	if len(key) == 32 {
		k = []byte(key)
	} else {
		h := sha256.Sum256([]byte(key))
		k = h[:]
	}

	// Validate the key by creating a cipher block once.
	if _, err := aes.NewCipher(k); err != nil {
		return nil, err
	}

	return &AES{
		key:         k,
		adminPwHash: adminPwHash,
	}, nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM and returns a
// base64-encoded ciphertext with the nonce prepended.
func (a *AES) Encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// Seal appends ciphertext to the nonce slice.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
//
// If the input is not valid base64, is too short to contain a nonce, or
// fails GCM authentication, the original string is returned unchanged.
// This provides backward compatibility with Stub-era plaintext values
// stored in the datasource table before encryption was enabled.
func (a *AES) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		// Not valid base64 — likely Stub-era plaintext; return as-is.
		return enc, nil
	}

	block, err := aes.NewCipher(a.key)
	if err != nil {
		return enc, nil
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return enc, nil
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		// Too short to contain a nonce — likely plaintext; return as-is.
		return enc, nil
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// GCM authentication failed — likely Stub-era plaintext; return as-is.
		return enc, nil
	}
	return string(plain), nil
}

// VerifyAdminPw checks the supplied management password against the stored
// bcrypt hash. Returns false if no hash is configured.
func (a *AES) VerifyAdminPw(input string) bool {
	if a.adminPwHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(a.adminPwHash), []byte(input)) == nil
}
