package pushrecorder

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Subscription is the PushSubscription-shaped value a hermetic end-to-end
// test feeds to the code under test, in place of a real subscription
// obtained from a browser's push manager. Its Endpoint points back at this
// fixture's own httptest server.
type Subscription struct {
	Endpoint string           `json:"endpoint"`
	Keys     SubscriptionKeys `json:"keys"`
}

// SubscriptionKeys carries the fixture's ECDH public key and auth secret,
// base64url-encoded exactly as a real PushSubscription.keys would be.
type SubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// VAPIDClaims is the JWT payload of a VAPID (RFC 8292) Authorization
// header: audience, expiry, and subject.
type VAPIDClaims struct {
	Audience string `json:"aud"`
	Expiry   int64  `json:"exp"`
	Subject  string `json:"sub"`
}

// VAPID is everything captured from a request's VAPID Authorization header.
type VAPID struct {
	Token          string      // the raw JWT, "t=" value
	Claims         VAPIDClaims // decoded JWT payload
	PublicKey      []byte      // raw uncompressed EC point, "k=" value
	SignatureValid bool        // whether the JWT signature verifies against PublicKey
}

// Request is one captured, decrypted POST to the fixture.
type Request struct {
	Headers http.Header
	VAPID   VAPID

	Salt            []byte // RFC 8188 aes128gcm header salt
	RecordSize      uint32 // RFC 8188 aes128gcm header record size
	SenderPublicKey []byte // raw uncompressed EC point, the "keyid" in the aes128gcm header

	Plaintext []byte // decrypted, de-padded payload

	// Err is non-nil if the VAPID header or the encryption envelope could
	// not be parsed, or decryption/authentication failed. A caller
	// asserting on a well-formed request should check this first.
	Err error
}

// Server is a recording Web Push endpoint. Construct one with NewServer,
// hand its Subscription to the code under test, and inspect Requests after
// it has sent a push message.
type Server struct {
	httpServer *httptest.Server
	priv       *ecdh.PrivateKey
	authSecret [16]byte

	mu       sync.Mutex
	requests []Request
}

// NewServer starts a recording push server. It generates a fresh ECDH P-256
// keypair and a random 16-byte auth secret on every call, so servers are
// never shared across tests.
func NewServer() *Server {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		// crypto/rand failing is not something a test can meaningfully
		// recover from.
		panic(fmt.Sprintf("pushrecorder: generating ECDH keypair: %v", err))
	}

	var authSecret [16]byte
	if _, err := io.ReadFull(rand.Reader, authSecret[:]); err != nil {
		panic(fmt.Sprintf("pushrecorder: generating auth secret: %v", err))
	}

	s := &Server{priv: priv, authSecret: authSecret}
	s.httpServer = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Close shuts down the underlying httptest server.
func (s *Server) Close() {
	s.httpServer.Close()
}

// URL is the fixture's own endpoint URL.
func (s *Server) URL() string {
	return s.httpServer.URL
}

// Subscription returns a PushSubscription-shaped value pointing at this
// fixture, suitable for handing to code that sends Web Push requests.
func (s *Server) Subscription() Subscription {
	return Subscription{
		Endpoint: s.httpServer.URL,
		Keys: SubscriptionKeys{
			P256dh: base64.RawURLEncoding.EncodeToString(s.priv.PublicKey().Bytes()),
			Auth:   base64.RawURLEncoding.EncodeToString(s.authSecret[:]),
		},
	}
}

// Requests returns every request captured so far, in arrival order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	req := Request{Headers: r.Header.Clone()}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		req.Err = fmt.Errorf("pushrecorder: reading body: %w", err)
	}

	if auth := req.Headers.Get("Authorization"); auth != "" {
		vapid, verr := parseVAPIDHeader(auth)
		if verr != nil {
			req.Err = errors.Join(req.Err, fmt.Errorf("pushrecorder: parsing VAPID header: %w", verr))
		} else {
			req.VAPID = vapid
		}
	}

	if err == nil {
		salt, recordSize, senderPub, ciphertext, perr := parseEnvelope(body)
		if perr != nil {
			req.Err = errors.Join(req.Err, fmt.Errorf("pushrecorder: parsing envelope: %w", perr))
		} else {
			req.Salt = salt
			req.RecordSize = recordSize
			req.SenderPublicKey = senderPub

			plaintext, derr := decryptAES128GCM(s.priv, s.authSecret[:], salt, senderPub, ciphertext)
			if derr != nil {
				req.Err = errors.Join(req.Err, fmt.Errorf("pushrecorder: decrypting: %w", derr))
			} else {
				req.Plaintext = plaintext
			}
		}
	}

	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
}

// parseVAPIDHeader parses an RFC 8292 Authorization header of the form
// `vapid t=<JWT>, k=<base64url ECDSA P-256 public key>`, decodes the JWT
// claims, and verifies the ES256 signature against the supplied key.
func parseVAPIDHeader(header string) (VAPID, error) {
	const prefix = "vapid "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return VAPID{}, fmt.Errorf("not a vapid scheme header: %q", header)
	}

	var token, key string
	for _, part := range strings.Split(header[len(prefix):], ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "t":
			token = strings.TrimSpace(kv[1])
		case "k":
			key = strings.TrimSpace(kv[1])
		}
	}
	if token == "" || key == "" {
		return VAPID{}, fmt.Errorf("missing t= or k= parameter: %q", header)
	}

	publicKey, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		return VAPID{}, fmt.Errorf("decoding k=: %w", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return VAPID{}, fmt.Errorf("not a JWT (expected 3 dot-separated parts): %q", token)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return VAPID{}, fmt.Errorf("decoding JWT payload: %w", err)
	}
	var claims VAPIDClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return VAPID{}, fmt.Errorf("unmarshalling JWT claims: %w", err)
	}

	sigValid := verifyES256(parts[0]+"."+parts[1], parts[2], publicKey)

	return VAPID{
		Token:          token,
		Claims:         claims,
		PublicKey:      publicKey,
		SignatureValid: sigValid,
	}, nil
}

// verifyES256 verifies a JWT ES256 signature (fixed-width r||s, 32 bytes
// each) against a raw uncompressed P-256 public key. Any malformed input
// yields false rather than an error, since an invalid signature is a fact
// about the captured request, not a fixture failure.
func verifyES256(signingInput, sigB64 string, rawPublicKey []byte) bool {
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), rawPublicKey)
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != 64 {
		return false
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	hash := sha256.Sum256([]byte(signingInput))
	return ecdsa.Verify(pub, hash[:], r, s)
}

// parseEnvelope splits an RFC 8188 aes128gcm body into its header fields
// and ciphertext: salt(16) || record_size(4, big-endian) || idlen(1) ||
// keyid(idlen) || ciphertext.
func parseEnvelope(body []byte) (salt []byte, recordSize uint32, senderPublicKey []byte, ciphertext []byte, err error) {
	const headerPrefixLen = 16 + 4 + 1
	if len(body) < headerPrefixLen {
		return nil, 0, nil, nil, fmt.Errorf("body too short for aes128gcm header: %d bytes", len(body))
	}

	salt = body[0:16]
	recordSize = binary.BigEndian.Uint32(body[16:20])
	idlen := int(body[20])

	if len(body) < headerPrefixLen+idlen {
		return nil, 0, nil, nil, fmt.Errorf("body too short for %d-byte keyid", idlen)
	}
	senderPublicKey = body[headerPrefixLen : headerPrefixLen+idlen]
	ciphertext = body[headerPrefixLen+idlen:]
	return salt, recordSize, senderPublicKey, ciphertext, nil
}
