package repo

import (
	"bytes"
	"testing"
)

func TestIdentityCodecEncryptsAndHashesDeterministically(t *testing.T) {
	codec, err := NewIdentityCodec([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	firstCipher, firstHash, err := codec.Encode("openid-sensitive")
	if err != nil {
		t.Fatal(err)
	}
	secondCipher, secondHash, err := codec.Encode("openid-sensitive")
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatal("lookup hash must be deterministic")
	}
	if bytes.Equal(firstCipher, secondCipher) {
		t.Fatal("ciphertext should use a random nonce")
	}
	decoded, err := codec.Decode(firstCipher)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != "openid-sensitive" {
		t.Fatalf("unexpected decoded identity %q", decoded)
	}
}
