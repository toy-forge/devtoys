package jwt

import (
	"fmt"
	"slices"
	"strings"
	"time"

	json "encoding/json/v2"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// claimOptions are independent of signature verification. The UI always
// reports the signature result separately from these claim comparisons.
type claimOptions struct {
	lifetime, issuer, audience	bool
	issuers, audiences		string
}

func (o claimOptions) enabled() bool	{ return o.lifetime || o.issuer || o.audience }
func expectedValues(text string) []string {
	var values []string
	for part := range strings.SplitSeq(text, ",") {
		if v := strings.TrimSpace(part); v != "" {
			values = append(values, v)
		}
	}
	return values
}
func checkClaims(payload []byte, opts claimOptions, now time.Time) string {
	if !opts.enabled() {
		return "No claim checks enabled."
	}
	var claims jwtlib.MapClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return "Claim checks: payload must be a JSON object"
	}
	lines := []string{"Claim checks (signature reported separately)"}
	result := func(name string, err error) {
		if err != nil {
			lines = append(lines, name+": FAIL — "+err.Error())
		} else {
			lines = append(lines, name+": PASS")
		}
	}
	if opts.lifetime {
		// Require exp, and check nbf when present, matching the desktop validator.
		v := jwtlib.NewValidator(jwtlib.WithExpirationRequired(), jwtlib.WithLeeway(5*time.Minute), jwtlib.WithTimeFunc(func() time.Time { return now }))
		err := v.Validate(claims)
		exp, expErr := claims.GetExpirationTime()
		nbf, nbfErr := claims.GetNotBefore()
		if err == nil && expErr == nil && nbfErr == nil && exp != nil && nbf != nil && nbf.After(exp.Time) {
			err = fmt.Errorf("not-before time is after expiration")
		}
		result("Lifetime", err)
		lines = append(lines, "Clock skew: 5 minutes")
	}
	if opts.issuer {
		expected := expectedValues(opts.issuers)
		issuer, err := claims.GetIssuer()
		if err == nil && (issuer == "" || !slices.Contains(expected, issuer)) {
			err = fmt.Errorf("issuer does not match the configured values")
		}
		result("Issuer", err)
	}
	if opts.audience {
		expected := expectedValues(opts.audiences)
		aud, err := claims.GetAudience()
		if err == nil {
			match := false
			for _, value := range aud {
				if slices.Contains(expected, value) {
					match = true
					break
				}
			}
			if !match {
				err = fmt.Errorf("audience does not match the configured values")
			}
		}
		result("Audience", err)
	}
	return strings.Join(lines, "\n")
}
