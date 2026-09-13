package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// recordSize is the RFC 8188 rs header field. A fixed, single-record
// payload never needs chunking, so this just has to exceed the record's
// own length; 4096 matches the value pushrecorder's fixture expects.
const recordSize = 4096

// encryptAES128GCM is the sender side of RFC 8291: a fresh ephemeral ECDH
// keypair per message, the same HKDF chain internal/testutil/pushrecorder's
// crypto.go decrypts (mirrored here, not rederived), AES-128-GCM sealing,
// and the RFC 8188 envelope: salt || rs || idlen || keyid || ciphertext.
func encryptAES128GCM(receiverPub *ecdh.PublicKey, authSecret, plaintext []byte) ([]byte, error) {
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

	// ikm = HKDF-Expand(HKDF-Extract(auth_secret, ecdh_secret), key_info, 32)
	prkKey := hkdfExtract(authSecret, sharedSecret)
	keyInfo := buildInfo("WebPush: info", receiverPubRaw, senderPubRaw)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	// PRK = HKDF-Extract(salt, ikm)
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
	// delimiter (0x02); a fixed payload needs no further padding.
	padded := append(append([]byte{}, plaintext...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

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

// hkdfExtract is RFC 5869's HKDF-Extract: PRK = HMAC-Hash(salt, IKM).
func hkdfExtract(salt, ikm []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

// hkdfExpand is RFC 5869's HKDF-Expand, specialised to a single output
// block (length <= 32), which is all RFC 8291 ever needs: T(1) =
// HMAC-Hash(PRK, info || 0x01), truncated to length.
func hkdfExpand(prk, info []byte, length int) []byte {
	mac := hmac.New(sha256.New, prk)
	mac.Write(info)
	mac.Write([]byte{0x01})
	return mac.Sum(nil)[:length]
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
