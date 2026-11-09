package cronparser

import (
	"testing"
	"time"
)

func TestNextOccurrences(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dates, err := nextOccurrences("*/15 * * * *", false, 3, now)
	if err != nil || len(dates) != 3 || !dates[0].Equal(now.Add(15*time.Minute)) || !dates[2].Equal(now.Add(45*time.Minute)) {
		t.Fatalf("dates=%v err=%v", dates, err)
	}
	dates, err = nextOccurrences("*/5 * * * * *", true, 2, now)
	if err != nil || !dates[1].Equal(now.Add(10*time.Second)) {
		t.Fatalf("seconds=%v err=%v", dates, err)
	}
	if _, err = nextOccurrences("99 * * * *", false, 1, now); err == nil {
		t.Fatal("invalid expression accepted")
	}
	if _, err = nextOccurrences("* * * * *", true, 1, now); err == nil {
		t.Fatal("missing seconds accepted")
	}
}
