package pushrecorder

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"
)

// mustB64 decodes an unpadded base64url string, exactly as RFC 8291
// Appendix A prints its fixed values. It panics on bad input, which is
// only ever a typo in the test's own literals.
func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("mustB64(%q): %v", s, err)
	}
	return b
}

// TestRFC8291WorkedExample reproduces the encryption example in RFC 8291
// Appendix A verbatim: fixed receiver ("user agent") and sender
// ("application server") keypairs, a fixed salt and auth secret, and the
// resulting on-the-wire aes128gcm body. It checks every intermediate value
// the RFC names (ECDH shared secret, PRK_key, IKM, PRK, CEK, NONCE) as well
// as the final plaintext, so a mistake in the derivation chain is pinned
// to the exact step that produced it rather than surfacing only as a
// decryption failure.
//
// This is the primary correctness gate for this fixture's crypto: it must
// pass, against the RFC's own fixed values, before this package is trusted
// to verify anything else.
func TestRFC8291WorkedExample(t *testing.T) {
	receiverPrivRaw := mustB64(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94")
	receiverPubRaw := mustB64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	senderPubRaw := mustB64(t, "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8")

	authSecret := mustB64(t, "BTBZMqHH6r4Tts7J_aSIgg")
	salt := mustB64(t, "DGv6ra1nlYgDCS1FRnbzlw")

	wantSharedSecret := mustB64(t, "kyrL1jIIOHEzg3sM2ZWRHDRB62YACZhhSlknJ672kSs")
	wantPRKKey := mustB64(t, "Snr3JMxaHVDXHWJn5wdC52WjpCtd2EIEGBykDcZW32k")
	wantIKM := mustB64(t, "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg")
	wantPRK := mustB64(t, "09_eUZGrsvxChDCGRCdkLiDXrReGOEVeSCdCcPBSJSc")
	wantCEK := mustB64(t, "oIhVW04MRdy2XN9CiKLxTg")
	wantNonce := mustB64(t, "4h_95klXJ5E_qnoN")
	wantPlaintext := mustB64(t, "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24")

	// The three base64url lines from RFC 8291 Appendix A's final HTTP
	// request body, concatenated (the RFC wraps them for the page).
	body := mustB64(t, strings.Join([]string{
		"DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml",
		"mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPT",
		"pK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN",
	}, ""))

	receiverPriv, err := ecdh.P256().NewPrivateKey(receiverPrivRaw)
	if err != nil {
		t.Fatalf("loading fixed receiver private key: %v", err)
	}
	if !bytes.Equal(receiverPriv.PublicKey().Bytes(), receiverPubRaw) {
		t.Fatalf("fixed receiver keypair is inconsistent: derived public key does not match RFC's")
	}

	salt2, recordSize, senderPub, ciphertext, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if !bytes.Equal(salt2, salt) {
		t.Errorf("salt parsed from body = %x, want %x", salt2, salt)
	}
	if recordSize != 4096 {
		t.Errorf("recordSize = %d, want 4096", recordSize)
	}
	if !bytes.Equal(senderPub, senderPubRaw) {
		t.Errorf("sender public key parsed from body = %x, want %x", senderPub, senderPubRaw)
	}

	// Step through the RFC 8291 §3.4 derivation chain, checking every
	// intermediate value against the RFC's own worked numbers.
	senderPubKey, err := ecdh.P256().NewPublicKey(senderPub)
	if err != nil {
		t.Fatalf("parsing sender public key: %v", err)
	}
	sharedSecret, err := receiverPriv.ECDH(senderPubKey)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}
	if !bytes.Equal(sharedSecret, wantSharedSecret) {
		t.Fatalf("ecdh shared secret = %x, want %x", sharedSecret, wantSharedSecret)
	}

	prkKey := hkdfExtract(authSecret, sharedSecret)
	if !bytes.Equal(prkKey, wantPRKKey) {
		t.Fatalf("PRK_key = %x, want %x", prkKey, wantPRKKey)
	}

	keyInfo := buildInfo("WebPush: info", receiverPriv.PublicKey().Bytes(), senderPub)
	ikm := hkdfExpand(prkKey, keyInfo, 32)
	if !bytes.Equal(ikm, wantIKM) {
		t.Fatalf("IKM = %x, want %x", ikm, wantIKM)
	}

	prk := hkdfExtract(salt, ikm)
	if !bytes.Equal(prk, wantPRK) {
		t.Fatalf("PRK = %x, want %x", prk, wantPRK)
	}

	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	if !bytes.Equal(cek, wantCEK) {
		t.Fatalf("CEK = %x, want %x", cek, wantCEK)
	}

	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)
	if !bytes.Equal(nonce, wantNonce) {
		t.Fatalf("NONCE = %x, want %x", nonce, wantNonce)
	}

	// And finally, the full decrypt path as a live server would run it.
	plaintext, err := decryptAES128GCM(receiverPriv, authSecret, salt, senderPub, ciphertext)
	if err != nil {
		t.Fatalf("decryptAES128GCM: %v", err)
	}
	if !bytes.Equal(plaintext, wantPlaintext) {
		t.Fatalf("plaintext = %q, want %q", plaintext, wantPlaintext)
	}
}
