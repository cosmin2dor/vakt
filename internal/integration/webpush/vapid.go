package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"time"
)

// vapidClaims is the JWT payload of an RFC 8292 VAPID Authorization header.
type vapidClaims struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
	Sub string `json:"sub"`
}

// buildVAPIDHeader signs an RFC 8292 VAPID JWT for one push-service
// request — aud is the endpoint's own origin, exp is bounded by ttl (must
// stay <=24h per the RFC), sub is the caller's mailto: contact — and
// returns the `vapid t=<jwt>, k=<pubkey>` Authorization header value.
func buildVAPIDHeader(priv *ecdsa.PrivateKey, pubKeyRaw []byte, endpoint, contact string, ttl time.Duration) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing endpoint: %w", err)
	}
	aud := u.Scheme + "://" + u.Host

	hdrJSON, err := json.Marshal(struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}{"JWT", "ES256"})
	if err != nil {
		return "", fmt.Errorf("encoding JWT header: %w", err)
	}
	claimsJSON, err := json.Marshal(vapidClaims{Aud: aud, Exp: time.Now().Add(ttl).Unix(), Sub: contact})
	if err != nil {
		return "", fmt.Errorf("encoding JWT claims: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(hdrJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	hash := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	// crypto/ecdsa doesn't normalize to low-S; some verifiers reject a
	// high-S signature even though JWS/RFC 8292 don't require low-S.
	// (r, n-s) verifies identically to (r, s), so this is a free, standard
	// normalization (BIP-62-style), not a different signature — cheap
	// defensive hardening even though it wasn't the cause of any specific
	// failure observed against a real push service.
	halfOrder := new(big.Int).Rsh(elliptic.P256().Params().N, 1)
	if s.Cmp(halfOrder) > 0 {
		s = new(big.Int).Sub(elliptic.P256().Params().N, s)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])

	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
	k := base64.RawURLEncoding.EncodeToString(pubKeyRaw)
	return fmt.Sprintf("vapid t=%s, k=%s", token, k), nil
}
