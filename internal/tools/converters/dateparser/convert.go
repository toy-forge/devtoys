package dateparser

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const unixEpochTicks int64 = 621355968000000000
const ticksPerSecond int64 = 10000000
const maxDateTicks int64 = 3155378975999999999

type timestampUnit int

const (
	seconds	timestampUnit	= iota
	milliseconds
	ticks
)

var unitLabels = []string{"Seconds", "Milliseconds", "Ticks"}

func ticksOf(t time.Time) (int64, error) {
	u := t.UTC()
	if u.Year() < 1 || u.Year() > 9999 {
		return 0, fmt.Errorf("date must be between years 1 and 9999")
	}
	return u.Unix()*ticksPerSecond + int64(u.Nanosecond()/100) + unixEpochTicks, nil
}
func validateInZone(t time.Time, loc *time.Location) error {
	if _, err := ticksOf(t); err != nil {
		return err
	}
	local := t.In(loc)
	if local.Year() < 1 || local.Year() > 9999 {
		return fmt.Errorf("date is outside years 1–9999 in this time zone")
	}
	return nil
}
func timestampValue(t, epoch time.Time, unit timestampUnit) (string, error) {
	value, err := ticksOf(t)
	if err != nil {
		return "", err
	}
	if unit == ticks {
		return strconv.FormatInt(value, 10), nil
	}
	base, err := ticksOf(epoch)
	if err != nil {
		return "", err
	}
	divisor := ticksPerSecond
	if unit == milliseconds {
		divisor = 10000
	}
	return strconv.FormatInt((value-base)/divisor, 10), nil
}
func timeFromTimestamp(text string, epoch time.Time, unit timestampUnit) (time.Time, error) {
	n, ok := new(big.Int).SetString(strings.TrimSpace(text), 10)
	if !ok {
		return time.Time{}, fmt.Errorf("timestamp must be an integer")
	}
	if unit != ticks {
		base, err := ticksOf(epoch)
		if err != nil {
			return time.Time{}, err
		}
		factor := ticksPerSecond
		if unit == milliseconds {
			factor = 10000
		}
		n.Mul(n, big.NewInt(factor))
		n.Add(n, big.NewInt(base))
	}
	if n.Sign() < 0 || n.Cmp(big.NewInt(maxDateTicks)) > 0 {
		return time.Time{}, fmt.Errorf("timestamp is outside years 1–9999")
	}
	count := n.Int64()
	secs, sub := count/ticksPerSecond, count%ticksPerSecond
	return time.Unix(secs-62135596800, sub*100).UTC(), nil
}
func parseDateTime(date, clock string, loc *time.Location) (time.Time, error) {
	value := strings.TrimSpace(date) + " " + strings.TrimSpace(clock)
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		wall, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		if wall.Year() < 1 || wall.Year() > 9999 {
			return time.Time{}, fmt.Errorf("date must be between years 1 and 9999")
		}
		t := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), loc)
		if t.Year() != wall.Year() || t.Month() != wall.Month() || t.Day() != wall.Day() || t.Hour() != wall.Hour() || t.Minute() != wall.Minute() || t.Second() != wall.Second() {
			return time.Time{}, fmt.Errorf("local time does not exist in this time zone")
		}
		if _, err := ticksOf(t); err != nil {
			return time.Time{}, err
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("use YYYY-MM-DD and HH:MM:SS")
}

// parseFlexibleDate parses common date layouts, interpreting zone-less
// dates in the selected time zone.
func parseFlexibleDate(text string, loc *time.Location) (time.Time, error) {
	value := strings.TrimSpace(text)
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, value, loc)
		if err == nil {
			hasZone := strings.Contains(layout, "MST") || strings.Contains(layout, "-07") || strings.Contains(layout, "Z07")
			if !hasZone {
				wall, wallErr := time.Parse(layout, value)
				if wallErr == nil && (t.Year() != wall.Year() || t.Month() != wall.Month() || t.Day() != wall.Day() || t.Hour() != wall.Hour() || t.Minute() != wall.Minute() || t.Second() != wall.Second()) {
					return time.Time{}, fmt.Errorf("local time does not exist in this time zone")
				}
			}
			if err = validateInZone(t, loc); err != nil {
				return time.Time{}, err
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("enter an ISO 8601 or recognized date")
}
func loadZone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "Local" {
		return time.Local, nil
	}
	if name == "" {
		return nil, fmt.Errorf("enter an IANA time zone")
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", name)
	}
	return loc, nil
}

// ambiguousLocal detects a repeated wall time around a zone transition.
func ambiguousLocal(t time.Time) bool {
	current := t.In(t.Location())
	_, offset := current.Zone()
	for _, delta := range []time.Duration{-48 * time.Hour, 48 * time.Hour} {
		_, otherOffset := t.Add(delta).In(t.Location()).Zone()
		if otherOffset == offset {
			continue
		}
		alternate := t.Add(time.Duration(offset-otherOffset) * time.Second).In(t.Location())
		if alternate.Year() == current.Year() && alternate.Month() == current.Month() && alternate.Day() == current.Day() && alternate.Hour() == current.Hour() && alternate.Minute() == current.Minute() && alternate.Second() == current.Second() {
			return true
		}
	}
	return false
}
