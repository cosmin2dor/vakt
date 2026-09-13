package pushrecorder

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestServerCapturesAndDecryptsLiveRequest builds a real Web Push HTTP
// request by hand — VAPID Authorization header, aes128gcm envelope, the
// works — the same shape internal/integration/webpush will send, POSTs it
// to a running Server, and asserts on the three things the task requires:
// headers, the encryption envelope's structural fields, and the decrypted
// payload.
func TestServerCapturesAndDecryptsLiveRequest(t *testing.T) {
	srv := NewServer()
	defer srv.Close()

	sub := srv.Subscription()
	if sub.Endpoint != srv.URL() {
		t.Fatalf("Subscription().Endpoint = %q, want %q", sub.Endpoint, srv.URL())
	}

	receiverPub, authSecret := decodeSubscription(t, sub)

	const plaintext = `{"title":"Take the bins out"}`
	body, err := encryptForTest(receiverPub, authSecret, []byte(plaintext))
	if err != nil {
		t.Fatalf("encryptForTest: %v", err)
	}

	authHeader, vapidPubRaw, err := buildVAPIDHeader("https://example.com", "mailto:ops@example.com")
	if err != nil {
		t.Fatalf("buildVAPIDHeader: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "60")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("posting to fixture: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("closing response body: %v", cerr)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("Requests() len = %d, want 1", len(reqs))
	}
	got := reqs[0]

	if got.Err != nil {
		t.Fatalf("captured request has unexpected Err: %v", got.Err)
	}

	// 1. Headers.
	if got.Headers.Get("TTL") != "60" {
		t.Errorf("TTL header = %q, want %q", got.Headers.Get("TTL"), "60")
	}
	if got.Headers.Get("Content-Encoding") != "aes128gcm" {
		t.Errorf("Content-Encoding header = %q, want %q", got.Headers.Get("Content-Encoding"), "aes128gcm")
	}
	if got.VAPID.Claims.Audience != "https://example.com" {
		t.Errorf("VAPID aud = %q, want %q", got.VAPID.Claims.Audience, "https://example.com")
	}
	if got.VAPID.Claims.Subject != "mailto:ops@example.com" {
		t.Errorf("VAPID sub = %q, want %q", got.VAPID.Claims.Subject, "mailto:ops@example.com")
	}
	if !bytes.Equal(got.VAPID.PublicKey, vapidPubRaw) {
		t.Errorf("VAPID public key mismatch")
	}
	if !got.VAPID.SignatureValid {
		t.Errorf("VAPID signature did not verify")
	}

	// 2. Encryption envelope.
	if len(got.Salt) != 16 {
		t.Errorf("Salt len = %d, want 16", len(got.Salt))
	}
	if got.RecordSize != 4096 {
		t.Errorf("RecordSize = %d, want 4096", got.RecordSize)
	}
	if len(got.SenderPublicKey) != 65 || got.SenderPublicKey[0] != 0x04 {
		t.Errorf("SenderPublicKey is not a 65-byte uncompressed EC point: %x", got.SenderPublicKey)
	}

	// 3. Decrypted payload.
	if string(got.Plaintext) != plaintext {
		t.Errorf("Plaintext = %q, want %q", got.Plaintext, plaintext)
	}
}

func decodeSubscription(t *testing.T, sub Subscription) (*ecdh.PublicKey, []byte) {
	t.Helper()
	rawPub, err := base64.RawURLEncoding.DecodeString(sub.Keys.P256dh)
	if err != nil {
		t.Fatalf("decoding p256dh: %v", err)
	}
	pub, err := ecdh.P256().NewPublicKey(rawPub)
	if err != nil {
		t.Fatalf("parsing p256dh: %v", err)
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(sub.Keys.Auth)
	if err != nil {
		t.Fatalf("decoding auth: %v", err)
	}
	return pub, authSecret
}

// encryptForTest is the sender side of RFC 8291: it plays the role
// internal/integration/webpush will eventually play, so this test can
// exercise the fixture's decryption against a request built the same way a
// real push service client builds one, rather than only against a
// pre-recorded golden vector.
func encryptForTest(receiverPub *ecdh.PublicKey, authSecret, plaintext []byte) ([]byte, error) {
	senderPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral sender key: %w", err)
	}
	senderPubRaw := senderPriv.PublicKey().Bytes()
	receiverPubRaw := receiverPub.Bytes()

	var salt [16]byte
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	sharedSecret, err := senderPriv.ECDH(receiverPub)
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}

	prkKey := hkdfExtract(authSecret, sharedSecret)
	keyInfo := buildInfo("WebPush: info", receiverPubRaw, senderPubRaw)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	prk := hkdfExtract(salt[:], ikm)
	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("constructing AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("constructing GCM: %w", err)
	}

	// Single record: plaintext followed by the RFC 8188 final-record
	// delimiter (0x02), no further padding.
	padded := append(append([]byte{}, plaintext...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	const recordSize = 4096
	body := make([]byte, 0, 16+4+1+len(senderPubRaw)+len(ciphertext))
	body = append(body, salt[:]...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], recordSize)
	body = append(body, rs[:]...)
	body = append(body, byte(len(senderPubRaw)))
	body = append(body, senderPubRaw...)
	body = append(body, ciphertext...)

	return body, nil
}

// buildVAPIDHeader builds an RFC 8292 `vapid t=<jwt>, k=<key>` Authorization
// header value with a freshly generated ECDSA P-256 signing key, and
// returns the header alongside the raw uncompressed public key it signed
// with, for the test to assert against.
func buildVAPIDHeader(audience, subject string) (header string, rawPublicKey []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("generating VAPID key: %w", err)
	}
	rawPublicKey, err = priv.PublicKey.Bytes()
	if err != nil {
		return "", nil, fmt.Errorf("encoding VAPID public key: %w", err)
	}

	type jwtHeader struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}
	hdrJSON, err := json.Marshal(jwtHeader{Typ: "JWT", Alg: "ES256"})
	if err != nil {
		return "", nil, err
	}
	claims := VAPIDClaims{
		Audience: audience,
		Expiry:   time.Now().Add(12 * time.Hour).Unix(),
		Subject:  subject,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", nil, err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(hdrJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	hash := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return "", nil, fmt.Errorf("signing JWT: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])

	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
	k := base64.RawURLEncoding.EncodeToString(rawPublicKey)

	return fmt.Sprintf("vapid t=%s, k=%s", token, k), rawPublicKey, nil
}
