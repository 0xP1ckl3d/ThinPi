package security

import (
	"bytes"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(h, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(h, "incorrect") {
		t.Fatal("invalid password accepted")
	}
}

func TestVaultRoundTripAndTamper(t *testing.T) {
	v, err := NewVault(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Decrypt(c)
	if err != nil || string(p) != "secret" {
		t.Fatalf("round trip: %q %v", p, err)
	}
	c[len(c)-1] ^= 1
	if _, err := v.Decrypt(c); err == nil {
		t.Fatal("tampering was accepted")
	}
}
