// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

func GenerateDeviceKeyPair() (publicKeyPEM, privateKeyPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate P-256 key: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshal PKCS#8 private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal SPKI public key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return string(pubPEM), string(privPEM), nil
}
func SignDeviceProof(privateKeyPEM, payload string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil || block.Type != "PRIVATE KEY" {
		return "", fmt.Errorf("device private key: bad PEM block %q", blockType(block))
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("device private key parse: %w", err)
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("device private key: %T is not an ECDSA key", key)
	}
	sum := sha256.Sum256([]byte(payload))
	sig, err := ecdsa.SignASN1(rand.Reader, ec, sum[:])
	if err != nil {
		return "", fmt.Errorf("device proof sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func blockType(b *pem.Block) string {
	if b == nil {
		return "<nil>"
	}
	return b.Type
}
