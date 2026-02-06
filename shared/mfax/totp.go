package mfax

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

func GenerateSecretBase32(bytesLen int) (string, error) {
	if bytesLen <= 0 {
		bytesLen = 20
	}
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// RFC recommends uppercase base32 without padding.
	return strings.ToUpper(base32NoPad.EncodeToString(b)), nil
}

func OTPAuthURL(issuer, account, secret string, digits int, period time.Duration) string {
	if digits <= 0 {
		digits = 6
	}
	if period <= 0 {
		period = 30 * time.Second
	}
	if issuer == "" {
		issuer = "esppd"
	}

	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", digits))
	q.Set("period", fmt.Sprintf("%d", int(period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func VerifyTOTP(secretBase32, code string, now time.Time, digits int, period time.Duration, window int) bool {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, " ", "")
	if code == "" {
		return false
	}
	secretBase32 = strings.TrimSpace(secretBase32)
	secretBase32 = strings.ReplaceAll(secretBase32, " ", "")
	if secretBase32 == "" {
		return false
	}

	if digits <= 0 {
		digits = 6
	}
	if period <= 0 {
		period = 30 * time.Second
	}
	if window < 0 {
		window = 0
	}

	for i := -window; i <= window; i++ {
		t := now.Add(time.Duration(i) * period)
		otp, err := totp(secretBase32, t, digits, period)
		if err == nil && otp == code {
			return true
		}
	}
	return false
}

func totp(secretBase32 string, t time.Time, digits int, period time.Duration) (string, error) {
	key, err := base32NoPad.DecodeString(strings.ToUpper(secretBase32))
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / int64(period.Seconds()))

	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)

	// Dynamic truncation.
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset : offset+4])
	bin &= 0x7fffffff

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	otp := bin % mod
	return fmt.Sprintf("%0*d", digits, otp), nil
}
