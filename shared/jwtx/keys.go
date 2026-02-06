package jwtx

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

func LoadPrivateKeyFromFile(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePrivateKeyPEMWithPassphrase(b, "")
}

func LoadPrivateKeyFromFileWithPassphrase(path, passphrase string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePrivateKeyPEMWithPassphrase(b, passphrase)
}

func LoadPublicKeyFromFile(path string) (*rsa.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePublicKeyPEM(b)
}

func ParsePrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	return ParsePrivateKeyPEMWithPassphrase(pemBytes, "")
}

func ParsePrivateKeyPEMWithPassphrase(pemBytes []byte, passphrase string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM")
	}

	der := block.Bytes
	if x509.IsEncryptedPEMBlock(block) {
		if passphrase == "" {
			return nil, errors.New("encrypted private key; set JWT_PRIVATE_KEY_PASSPHRASE")
		}
		var err error
		der, err = x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("decrypt private key: %w", err)
		}
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(der)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("unexpected private key type %T", k)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported private key PEM type %q", block.Type)
	}
}

func ParsePublicKeyPEM(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	switch block.Type {
	case "PUBLIC KEY":
		k, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("unexpected public key type %T", k)
		}
		return rsaKey, nil
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported public key PEM type %q", block.Type)
	}
}

func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}
	return priv, &priv.PublicKey, nil
}

func MarshalPrivateKeyPEM(priv *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
}

func MarshalPublicKeyPEM(pub *rsa.PublicKey) ([]byte, error) {
	b, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}), nil
}
