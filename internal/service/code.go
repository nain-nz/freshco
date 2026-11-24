package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const (
	claimCodeLen  = 12
	inviteCodeLen = 8
)

func randomCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = codeAlphabet[int(v)&31]
	}
	return string(out), nil
}

func formatCode(c string) string {
	var sb strings.Builder
	for i, r := range c {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func normalizeCode(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	c = strings.ReplaceAll(c, "-", "")
	return strings.ReplaceAll(c, " ", "")
}

func hashCode(normalized string) string {
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

type attemptLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[int][]time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{max: max, window: window, hits: map[int][]time.Time{}}
}

func (l *attemptLimiter) Allow(userID int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := l.hits[userID][:0]
	for _, t := range l.hits[userID] {
		if now.Sub(t) < l.window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= l.max {
		l.hits[userID] = keep
		return false
	}
	l.hits[userID] = append(keep, now)
	return true
}
