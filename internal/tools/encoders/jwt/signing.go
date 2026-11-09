package jwt

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

var algorithms = []string{"HS256", "HS384", "HS512", "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512"}

func signingMethod(alg string) (jwtlib.SigningMethod, error) {
	for _, allowed := range algorithms {
		if alg == allowed {
			return jwtlib.GetSigningMethod(alg), nil
		}
	}
	return nil, fmt.Errorf("unsupported signing algorithm %q", alg)
}

func signingKey(alg, key string, private, base64Key bool) (any, error) {
	if strings.HasPrefix(alg, "HS") {
		if base64Key {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
			if err != nil {
				return nil, fmt.Errorf("invalid Base64 signing key")
			}
			return b, nil
		}
		return []byte(key), nil
	}
	if strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS") {
		if private {
			return jwtlib.ParseRSAPrivateKeyFromPEM([]byte(key))
		}
		return jwtlib.ParseRSAPublicKeyFromPEM([]byte(key))
	}
	if private {
		return jwtlib.ParseECPrivateKeyFromPEM([]byte(key))
	}
	return jwtlib.ParseECPublicKeyFromPEM([]byte(key))
}

func signToken(headerText, payloadText, alg, key string, base64Key bool) (string, error) {
	return signTokenWithClaims(headerText, payloadText, alg, key, base64Key, encoderClaims{}, time.Now())
}

func signTokenWithClaims(headerText, payloadText, alg, key string, base64Key bool, claims encoderClaims, now time.Time) (string, error) {
	const maxInputBytes = 1 << 20
	if len(headerText) > maxInputBytes || len(payloadText) > maxInputBytes || len(key) > maxInputBytes {
		return "", fmt.Errorf("header, payload and key must each be at most 1 MiB")
	}
	method, err := signingMethod(alg)
	if err != nil {
		return "", err
	}
	var header, payload map[string]jsontext.Value
	if err := json.Unmarshal([]byte(headerText), &header); err != nil || header == nil {
		return "", fmt.Errorf("header must be a JSON object")
	}
	if err := json.Unmarshal([]byte(payloadText), &payload); err != nil || payload == nil {
		return "", fmt.Errorf("payload must be a JSON object")
	}
	if err := applyEncoderClaims(payload, claims, now); err != nil {
		return "", err
	}
	if key == "" {
		return "", fmt.Errorf("a signing key is required")
	}
	rawAlg, _ := json.Marshal(alg)
	header["alg"] = rawAlg
	head, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	material, err := signingKey(alg, key, true, base64Key)
	if err != nil {
		return "", fmt.Errorf("invalid signing key: %w", err)
	}
	input := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	sig, err := method.Sign(input, material)
	if err != nil {
		return "", fmt.Errorf("signing failed: %w", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func verifySignature(parts []string, alg, key string, base64Key bool) error {
	method, err := signingMethod(alg)
	if err != nil {
		return err
	}
	material, err := signingKey(alg, key, false, base64Key)
	if err != nil {
		return fmt.Errorf("invalid verification key: %w", err)
	}
	if ecMethod, ok := method.(*jwtlib.SigningMethodECDSA); ok {
		ecKey, ok := material.(*ecdsa.PublicKey)
		if !ok || ecKey.Curve.Params().BitSize != ecMethod.CurveBits {
			return fmt.Errorf("public key curve does not match %s", alg)
		}
	}
	sig, err := decodeSegment(parts[2])
	if err != nil {
		return fmt.Errorf("signature is not valid Base64URL")
	}
	return method.Verify(parts[0]+"."+parts[1], sig, material)
}
