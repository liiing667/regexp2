package regexp2

import (
	"errors"
	"testing"
	"time"
)

// TestFindNextMatchSharesTraversalDeadline verifies that continuing a
// traversal cannot reset its timeout budget, while an independent search
// starts a new one.
func TestFindNextMatchSharesTraversalDeadline(t *testing.T) {
	StopTimeoutClock()
	t.Cleanup(func() {
		StopTimeoutClock()
		fast.mu.Lock()
		fast.current.write(0)
		fast.clockEnd.write(0)
		fast.start = time.Time{}
		fast.running = false
		fast.mu.Unlock()
	})

	for _, tc := range []struct {
		name  string
		start func(*Regexp) (*Match, error)
	}{
		{
			name: "string",
			start: func(re *Regexp) (*Match, error) {
				return re.FindStringMatch("aaaa")
			},
		},
		{
			name: "runes",
			start: func(re *Regexp) (*Match, error) {
				return re.FindRunesMatch([]rune("aaaa"))
			},
		},
		{
			name: "string starting at",
			start: func(re *Regexp) (*Match, error) {
				return re.FindStringMatchStartingAt("xaaaa", 1)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			StopTimeoutClock()
			fast.mu.Lock()
			fast.start = time.Now().Add(-time.Minute)
			fast.current.write(0)
			fast.clockEnd.write(0)
			fast.running = false
			fast.mu.Unlock()

			re := MustCompile("a")
			re.MatchTimeout = time.Second

			m, err := tc.start(re)
			if err != nil {
				t.Fatalf("initial search: %v", err)
			}
			if m == nil {
				t.Fatal("initial search returned no match")
			}
			if !m.text.deadlineSet {
				t.Fatal("initial search did not start a traversal deadline")
			}
			deadline := m.text.deadline

			// Stop the clock and move it exactly to the traversal deadline.
			// The next continuation must fail rather than start a new budget.
			StopTimeoutClock()
			fast.mu.Lock()
			fast.current.write(deadline)
			fast.mu.Unlock()

			for attempt := 0; attempt < 2; attempt++ {
				next, err := re.FindNextMatch(m)
				if !errors.Is(err, ErrMatchTimeout) {
					t.Fatalf("attempt %d: err = %v, want ErrMatchTimeout", attempt, err)
				}
				if next != nil {
					t.Fatalf("attempt %d: returned match %v after traversal deadline", attempt, next)
				}
			}

			// A new initial search is a new traversal and gets a fresh budget,
			// even though the old traversal's deadline has passed.
			fresh, err := tc.start(re)
			if err != nil {
				t.Fatalf("independent search: %v", err)
			}
			if fresh == nil {
				t.Fatal("independent search returned no match")
			}
			if fresh.text == m.text {
				t.Fatal("independent search reused the old traversal text")
			}
			if !fresh.text.deadlineSet {
				t.Fatal("independent search did not start a new deadline")
			}
			next, err := re.FindNextMatch(fresh)
			if err != nil {
				t.Fatalf("independent traversal continuation: %v", err)
			}
			if next == nil {
				t.Fatal("independent traversal continuation returned no match")
			}
		})
	}
}
