package jwt

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"strings"
	"time"
)

type encoderClaims struct {
	issuerEnabled, audienceEnabled, expirationEnabled, defaultTimes	bool
	issuer, audience						string
	expiration							time.Time
	expirationSet							bool
}

// applyEncoderClaims only overrides explicitly enabled fields. Default times
// fill missing claims; a configured expiration takes precedence.
func applyEncoderClaims(payload map[string]jsontext.Value, opts encoderClaims, now time.Time) error {
	set := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		payload[key] = raw
		return nil
	}
	if opts.issuerEnabled {
		issuer := strings.TrimSpace(opts.issuer)
		if issuer == "" {
			return fmt.Errorf("enter an issuer")
		}
		if err := set("iss", issuer); err != nil {
			return err
		}
	}
	if opts.audienceEnabled {
		audience := strings.TrimSpace(opts.audience)
		if audience == "" {
			return fmt.Errorf("enter an audience")
		}
		if err := set("aud", audience); err != nil {
			return err
		}
	}
	if opts.expirationEnabled {
		if !opts.expirationSet || opts.expiration.Year() < 1 || opts.expiration.Year() > 9999 {
			return fmt.Errorf("enter a valid expiration date")
		}
		if err := set("exp", opts.expiration.Unix()); err != nil {
			return err
		}
	}
	if opts.defaultTimes {
		for _, entry := range []struct {
			key	string
			value	int64
		}{{"iat", now.Unix()}, {"nbf", now.Unix()}, {"exp", now.Add(time.Hour).Unix()}} {
			if _, exists := payload[entry.key]; !exists {
				if err := set(entry.key, entry.value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
