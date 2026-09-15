package webpush

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
)

// vapidTTL and pushTTL bound the VAPID JWT expiry (RFC 8292 caps it at
// 24h) and the TTL header telling the push service how long to hold an
// undelivered message.
const (
	vapidTTL = 12 * time.Hour
	pushTTL  = 60 * time.Second
)

// SubscriptionSource enumerates currently-enrolled push subscribers.
// *config.Store satisfies this directly via its List method.
type SubscriptionSource interface {
	List() []config.PushSubscription
}

// SubscriptionPruner removes a dead subscription by endpoint (SDD.md G14).
// *config.Store satisfies this directly via its Remove method.
type SubscriptionPruner interface {
	Remove(endpoint string) error
}

// Module is the ios_notifications dispatch.Module: VAPID signing (RFC
// 8292), aes128gcm payload encryption (RFC 8291), and HTTP delivery to
// every currently-enrolled subscription.
type Module struct {
	privateKey   *ecdsa.PrivateKey
	publicKeyRaw []byte
	contact      string
	subs         SubscriptionSource
	pruner       SubscriptionPruner
	httpClient   *http.Client
}

// New constructs a Module. privateKey and contact are the VAPID keypair
// and "sub" claim (a mailto: string) as plain parameters — this
// constructor takes no dependency on how or where that key material is
// persisted; sourcing it from real storage and registering the module
// with cmd/vaktd is a follow-up integration step. subs enumerates
// enrolled subscriptions on every Dispatch. pruner, if non-nil, is used
// to remove a subscription whose endpoint returns 404/410 (SDD.md G14);
// a nil pruner just skips pruning, though production wiring should
// always supply one (e.g. the same *config.Store as subs). httpClient
// defaults to a bounded client when nil, so a caller can inject one
// pointed at a test server.
func New(privateKey *ecdsa.PrivateKey, contact string, subs SubscriptionSource, pruner SubscriptionPruner, httpClient *http.Client) (*Module, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("webpush: private key must not be nil")
	}
	if subs == nil {
		return nil, fmt.Errorf("webpush: subscription source must not be nil")
	}
	pubKeyRaw, err := privateKey.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("webpush: encoding VAPID public key: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Module{
		privateKey:   privateKey,
		publicKeyRaw: pubKeyRaw,
		contact:      contact,
		subs:         subs,
		pruner:       pruner,
		httpClient:   httpClient,
	}, nil
}

// Name identifies this module in the dispatch registry.
func (m *Module) Name() string { return "ios_notifications" }

// Dispatch sends payload, verbatim, to every enrolled subscription. Per
// SDD.md G14, the task only fails if none accept — one dead phone must
// not mark the household's reminders broken.
func (m *Module) Dispatch(ctx context.Context, _ dispatch.TaskContext, payload string) (dispatch.Outcome, error) {
	subs := m.subs.List()
	if len(subs) == 0 {
		return dispatch.Outcome{Accepted: false, Detail: "0/0 accepted: no subscriptions enrolled"}, nil
	}

	accepted := 0
	var failures []string
	for _, sub := range subs {
		if err := m.sendOne(ctx, sub, payload); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		accepted++
	}

	detail := fmt.Sprintf("%d/%d accepted", accepted, len(subs))
	if len(failures) > 0 {
		detail += "; " + strings.Join(failures, ", ")
	}
	return dispatch.Outcome{Accepted: accepted > 0, Detail: detail}, nil
}

// sendOne encrypts payload for one subscriber and POSTs it to their
// endpoint. A non-nil error names the subscriber and describes the
// failure, for aggregation into Dispatch's Detail string.
func (m *Module) sendOne(ctx context.Context, sub config.PushSubscription, payload string) error {
	receiverPub, authSecret, err := decodeSubscription(sub)
	if err != nil {
		return fmt.Errorf("%s: %w", shortEndpoint(sub.Endpoint), err)
	}

	body, err := encryptAES128GCM(receiverPub, authSecret, []byte(payload))
	if err != nil {
		return fmt.Errorf("%s: encrypting payload: %w", shortEndpoint(sub.Endpoint), err)
	}

	authHeader, err := buildVAPIDHeader(m.privateKey, m.publicKeyRaw, sub.Endpoint, m.contact, vapidTTL)
	if err != nil {
		return fmt.Errorf("%s: signing VAPID header: %w", shortEndpoint(sub.Endpoint), err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: building request: %w", shortEndpoint(sub.Endpoint), err)
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(pushTTL.Seconds())))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", shortEndpoint(sub.Endpoint), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			return fmt.Errorf("%s: returned %d, %s", shortEndpoint(sub.Endpoint), resp.StatusCode, m.pruneNote(sub.Endpoint))
		}
		return fmt.Errorf("%s: returned %d", shortEndpoint(sub.Endpoint), resp.StatusCode)
	}
	return nil
}

// pruneNote removes endpoint via m.pruner (G14) if one is configured, and
// describes the outcome for sendOne's aggregated error/Detail string.
func (m *Module) pruneNote(endpoint string) string {
	if m.pruner == nil {
		return "not pruned: no pruner configured"
	}
	if err := m.pruner.Remove(endpoint); err != nil {
		return fmt.Sprintf("pruning failed: %v", err)
	}
	return "pruned"
}

// decodeSubscription parses a stored subscription's base64url-encoded
// ECDH public key and auth secret into the forms encryptAES128GCM needs.
func decodeSubscription(sub config.PushSubscription) (*ecdh.PublicKey, []byte, error) {
	rawPub, err := base64.RawURLEncoding.DecodeString(sub.Keys.P256dh)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding p256dh: %w", err)
	}
	pub, err := ecdh.P256().NewPublicKey(rawPub)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing p256dh: %w", err)
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(sub.Keys.Auth)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding auth: %w", err)
	}
	return pub, authSecret, nil
}

// shortEndpoint trims a subscription endpoint to its host for Detail
// strings, since a full endpoint URL can carry an opaque per-subscriber
// token that shouldn't be echoed back verbatim.
func shortEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}
	return u.Host
}
