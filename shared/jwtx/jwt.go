package jwtx

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID    int64  `json:"uid"`
	Role      string `json:"role,omitempty"`
	TokenType string `json:"token_type"`
}

type Signer struct {
	priv       *rsa.PrivateKey
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type Verifier struct {
	pub      *rsa.PublicKey
	issuer   string
	audience string
}

func NewSigner(priv *rsa.PrivateKey, issuer, audience string, accessTTL, refreshTTL time.Duration) *Signer {
	return &Signer{priv: priv, issuer: issuer, audience: audience, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func NewVerifier(pub *rsa.PublicKey, issuer, audience string) *Verifier {
	return &Verifier{pub: pub, issuer: issuer, audience: audience}
}

func NewSignerFromFile(privateKeyPath, issuer, audience string, accessTTL, refreshTTL time.Duration) (*Signer, error) {
	priv, err := LoadPrivateKeyFromFile(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return NewSigner(priv, issuer, audience, accessTTL, refreshTTL), nil
}

func NewSignerFromFileWithPassphrase(privateKeyPath, passphrase, issuer, audience string, accessTTL, refreshTTL time.Duration) (*Signer, error) {
	priv, err := LoadPrivateKeyFromFileWithPassphrase(privateKeyPath, passphrase)
	if err != nil {
		return nil, err
	}
	return NewSigner(priv, issuer, audience, accessTTL, refreshTTL), nil
}

func NewVerifierFromFile(publicKeyPath, issuer, audience string) (*Verifier, error) {
	pub, err := LoadPublicKeyFromFile(publicKeyPath)
	if err != nil {
		return nil, err
	}
	return NewVerifier(pub, issuer, audience), nil
}

func (s *Signer) NewAccessToken(userID int64, role string, tokenID string) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			ID:        tokenID,
		},
		UserID:    userID,
		Role:      role,
		TokenType: TokenTypeAccess,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(s.priv)
}

func (s *Signer) NewRefreshToken(userID int64, tokenID string) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
			ID:        tokenID,
		},
		UserID:    userID,
		TokenType: TokenTypeRefresh,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(s.priv)
}

func (v *Verifier) ParseAndValidate(tokenStr string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	parsed, err := parser.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		return v.pub, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}

	// Manual validations to keep behavior explicit.
	if claims.Issuer != v.issuer {
		return nil, fmt.Errorf("invalid issuer")
	}
	if !audContains(claims.Audience, v.audience) {
		return nil, fmt.Errorf("invalid audience")
	}
	if claims.ExpiresAt == nil || time.Now().After(claims.ExpiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}
	if claims.NotBefore != nil && time.Now().Before(claims.NotBefore.Time) {
		return nil, fmt.Errorf("token not valid yet")
	}

	return claims, nil
}

func audContains(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}
