package crypto

import (
	"bytes"
	"testing"
)

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	k, err := New(bytes.Repeat([]byte{7}, 32), "pepper")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	k := testKeyring(t)
	for _, in := range []string{"254708374149", "A012345678Z", "x"} {
		blob, err := k.EncryptString(in)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(blob, []byte(in)) {
			t.Fatalf("ciphertext leaks plaintext %q", in)
		}
		out, err := k.DecryptString(blob)
		if err != nil {
			t.Fatal(err)
		}
		if out != in {
			t.Fatalf("got %q want %q", out, in)
		}
	}
}

func TestEmptyAndMalformed(t *testing.T) {
	k := testKeyring(t)
	blob, err := k.Encrypt(nil)
	if err != nil || blob != nil {
		t.Fatalf("empty plaintext: blob=%v err=%v", blob, err)
	}
	if _, err := k.Decrypt([]byte{0x01, 0x02}); err == nil {
		t.Fatal("expected malformed error")
	}
	blob, _ = k.EncryptString("hello")
	blob[len(blob)-1] ^= 0xff
	if _, err := k.Decrypt(blob); err == nil {
		t.Fatal("expected authentication failure")
	}
}

func TestWrongMasterKey(t *testing.T) {
	k1 := testKeyring(t)
	k2, _ := New(bytes.Repeat([]byte{9}, 32), "pepper")
	blob, _ := k1.EncryptString("secret")
	if _, err := k2.Decrypt(blob); err == nil {
		t.Fatal("decrypt with wrong master key must fail")
	}
}

func TestHashIsDeterministicAndPeppered(t *testing.T) {
	k1 := testKeyring(t)
	k2, _ := New(bytes.Repeat([]byte{7}, 32), "other")
	a := k1.HashMSISDN("0708 374 149")
	b := k1.HashMSISDN("+254708374149")
	if !bytes.Equal(a, b) {
		t.Fatal("normalised numbers must hash equal")
	}
	if bytes.Equal(a, k2.HashMSISDN("0708374149")) {
		t.Fatal("different pepper must produce a different hash")
	}
	if k1.Hash("") != nil {
		t.Fatal("empty value hashes to nil")
	}
}

func TestNormaliseMSISDN(t *testing.T) {
	cases := map[string]string{
		"254708374149":    "254708374149",
		"+254 708 374149": "254708374149",
		"0708374149":      "254708374149",
		"0110000000":      "254110000000",
		"708374149":       "254708374149",
	}
	for in, want := range cases {
		got, err := NormaliseMSISDN(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q err %v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "12345", "255700000000", "0800000000"} {
		if _, err := NormaliseMSISDN(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
