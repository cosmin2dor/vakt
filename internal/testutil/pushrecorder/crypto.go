package pushrecorder

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

// decryptAES128GCM reverses RFC 8291 §3.4's message encryption, given the
// receiver's (this fixture's) ECDH private key, the auth secret from its
// subscription, and the salt/sender-public-key/ciphertext read off the
// wire by parseEnvelope.
func decryptAES128GCM(receiverPriv *ecdh.PrivateKey, authSecret, salt, senderPublicKey, ciphertext []byte) ([]byte, error) {
	senderPub, err := ecdh.P256().NewPublicKey(senderPublicKey)
	if err != nil {
		return nil, fmt.Errorf("parsing sender public key: %w", err)
	}

	sharedSecret, err := receiverPriv.ECDH(senderPub)
	if err != nil {
		return nil, fmt.Errorf("computing ECDH shared secret: %w", err)
	}

	receiverPub := receiverPriv.PublicKey().Bytes() // uncompressed, 65 bytes

	// ikm = HKDF-Expand(HKDF-Extract(auth_secret, ecdh_secret), key_info, 32)
	prkKey := hkdfExtract(authSecret, sharedSecret)
	keyInfo := buildInfo("WebPush: info", receiverPub, senderPublicKey)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	// PRK = HKDF-Extract(salt, ikm)
	prk := hkdfExtract(salt, ikm)

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

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM authentication failed: %w", err)
	}

	return stripRecordPadding(plaintext)
}

// buildInfo builds the RFC 8291 §3.4 key_info: "WebPush: info" || 0x00 ||
// receiver_public_key || sender_public_key.
func buildInfo(label string, receiverPublicKey, senderPublicKey []byte) []byte {
	info := make([]byte, 0, len(label)+1+len(receiverPublicKey)+len(senderPublicKey))
	info = append(info, label...)
	info = append(info, 0x00)
	info = append(info, receiverPublicKey...)
	info = append(info, senderPublicKey...)
	return info
}

// stripRecordPadding removes RFC 8188 record padding: scanning back from
// the end of the record for the first non-zero byte, which must be the
// delimiter (0x02 for a final/only record). Anything else is a decryption
// or padding error.
func stripRecordPadding(record []byte) ([]byte, error) {
	i := len(record) - 1
	for i >= 0 && record[i] == 0x00 {
		i--
	}
	if i < 0 {
		return nil, fmt.Errorf("record padding: no delimiter byte found")
	}
	if record[i] != 0x02 {
		return nil, fmt.Errorf("record padding: unexpected delimiter 0x%02x, want 0x02 (final record)", record[i])
	}
	return record[:i], nil
}

// hkdfExtract is RFC 5869's HKDF-Extract: PRK = HMAC-Hash(salt, IKM).
func hkdfExtract(salt, ikm []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

// hkdfExpand is RFC 5869's HKDF-Expand, specialised to a single output
// block (length <= 32, the SHA-256 output size), which is all RFC 8291
// ever asks for: T(1) = HMAC-Hash(PRK, info || 0x01), truncated to length.
func hkdfExpand(prk, info []byte, length int) []byte {
	if length > sha256.Size {
		panic(fmt.Sprintf("pushrecorder: hkdfExpand length %d exceeds single-block maximum %d", length, sha256.Size))
	}
	mac := hmac.New(sha256.New, prk)
	mac.Write(info)
	mac.Write([]byte{0x01})
	return mac.Sum(nil)[:length]
}
