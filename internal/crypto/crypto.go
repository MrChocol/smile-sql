// Package crypto defines the encryption / decryption / admin-password-verification
// contract used throughout the platform.
//
// Sprint 0 ships a Stub that passes values through unchanged and always
// verifies the admin password as true. Later sprints will replace Stub with a
// real AES implementation that reads the key from Settings.
package crypto

// Crypto is the contract every crypto implementation must satisfy.
type Crypto interface {
	// Encrypt encrypts a plaintext string (e.g. datasource password) and
	// returns the ciphertext suitable for TEXT-column storage.
	Encrypt(plain string) (string, error)

	// Decrypt decrypts a ciphertext previously produced by Encrypt.
	Decrypt(enc string) (string, error)

	// VerifyAdminPw checks the supplied management password against the
	// stored hash. Used when revealing datasource passwords in the UI.
	VerifyAdminPw(input string) bool
}

// Stub is a no-op Crypto used during Sprint 0 so the platform boots end-to-end.
//
// Encrypt returns the plaintext unchanged; Decrypt returns the ciphertext
// unchanged; VerifyAdminPw always returns true.
type Stub struct{}

// Ensure Stub satisfies Crypto at compile time.
var _ Crypto = (*Stub)(nil)

// Encrypt implements Crypto.
func (s *Stub) Encrypt(plain string) (string, error) { return plain, nil }

// Decrypt implements Crypto.
func (s *Stub) Decrypt(enc string) (string, error) { return enc, nil }

// VerifyAdminPw implements Crypto.
func (s *Stub) VerifyAdminPw(input string) bool { return true }
