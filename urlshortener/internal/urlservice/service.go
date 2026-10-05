// Package urlservice contains the business logic for creating and resolving
// short URLs. It depends on the repository abstraction (DI), not GORM directly.
package urlservice

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/yazdanjavadi/urlshort/internal/cache"
	"github.com/yazdanjavadi/urlshort/internal/repository"
	"github.com/yazdanjavadi/urlshort/internal/worker"
)

const (
	// codeAlphabet uses unambiguous characters (no 0/O, 1/I/l) for safe manual entry.
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	// maxAttempts bounds the retry loop when a generated code collides.
	maxAttempts = 1000
)

// Service creates and resolves short URLs.
type Service struct {
	repo     repository.Repository
	cache    cache.Cache
	log      *logrus.Entry
	codeLen  int
	cacheTTL time.Duration
	pool     worker.Submitter // nil => synchronous inserts
}

// New returns a Service wired to the given repository and cache. The cache is
// consulted on Resolve (read-through). Pass cache.Noop{} to disable caching.
// pool may be nil to keep inserts synchronous.
func New(
	repo repository.Repository,
	c cache.Cache,
	log *logrus.Entry,
	codeLen int,
	cacheTTL time.Duration,
	pool worker.Submitter,
) *Service {
	if codeLen <= 0 {
		codeLen = 6
	}

	if c == nil {
		c = cache.Noop{}
	}

	return &Service{repo: repo, cache: c, log: log, codeLen: codeLen, cacheTTL: cacheTTL, pool: pool}
}

// Create stores longURL under a fresh random code and returns it. If the long
// URL already has a code, the existing code is returned (dedup).
//
// When a worker pool is configured, the actual DB insert is performed
// asynchronously by a worker: Create enqueues the job and returns the code
// immediately. The caller assumes the insert will succeed (see step 4 note).
// Without a pool, the insert is synchronous.
func (s *Service) Create(ctx context.Context, longURL string) (string, error) {
	// Dedup: reuse existing code for the same long URL.
	if existing, err := s.repo.FindByLongURL(ctx, longURL); err == nil {
		s.log.WithFields(logrus.Fields{
			"long_url": longURL,
			"code":     existing.Code,
		}).Debug("long url already stored, reusing existing code")

		return existing.Code, nil
	} else if err != repository.ErrNotFound {
		return "", err
	}

	// Mint a fresh code. With async inserts we cannot retry on DB-side
	// duplicate-key collisions (the insert hasn't happened yet when we return),
	// so we generate until we find a code not already in flight. In practice
	// collisions on a 6-char crypto-random code are negligible.
	code, err := randomCode(s.codeLen)
	if err != nil {
		return "", err
	}

	if s.pool != nil {
		// Async: enqueue and return immediately.
		if err := s.pool.Submit(code, longURL); err != nil {
			return "", err
		}

		s.log.WithFields(logrus.Fields{
			"long_url": longURL,
			"code":     code,
		}).Info("enqueued async insert")

		return code, nil
	}

	// Sync path (no pool): insert now with collision retry.
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt == 0 {
			// reuse first code generated above
		} else {
			code, err = randomCode(s.codeLen)
			if err != nil {
				return "", err
			}
		}

		if err := s.repo.Save(ctx, code, longURL); err != nil {
			if err == repository.ErrDuplicateCode {
				s.log.WithField("attempt", attempt).Debug("code collision, retrying")

				continue
			}

			return "", err
		}

		s.log.WithFields(logrus.Fields{
			"long_url": longURL,
			"code":     code,
		}).Info("stored new short url")

		return code, nil
	}

	return "", ErrCodeGeneration
}

// Resolve returns the long URL for a code. It checks the cache first
// (read-through); on a miss it queries the repository and back-fills the cache.
func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	// 1. Try the cache.
	if cached, err := s.cache.Get(ctx, code); err == nil {
		s.log.WithField("code", code).Debug("cache hit")

		return cached, nil
	} else if err != cache.ErrCacheMiss {
		// A real cache error is logged but we fall through to the DB rather than
		// failing the request — the DB is the source of truth.
		s.log.WithError(err).WithField("code", code).Warn("cache get error, falling back to db")
	}

	// 2. Cache miss (or error) -> query the repository.
	rec, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return "", err
	}

	// 3. Back-fill the cache (best-effort; a failure here is non-fatal).
	if err := s.cache.Set(ctx, code, rec.LongURL, s.cacheTTL); err != nil {
		s.log.WithError(err).WithField("code", code).Warn("cache set error")
	}

	return rec.LongURL, nil
}

// randomCode builds a single random code of length n using crypto/rand.
func randomCode(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}

		b[i] = codeAlphabet[idx.Int64()]
	}

	return string(b), nil
}
