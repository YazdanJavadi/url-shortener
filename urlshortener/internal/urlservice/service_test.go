package urlservice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/yazdanjavadi/urlshort/internal/cache"
	"github.com/yazdanjavadi/urlshort/internal/models"
	"github.com/yazdanjavadi/urlshort/internal/repository"
)

func testLogger() *logrus.Entry {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)

	return logrus.NewEntry(l)
}

const (
	testCodeLen  = 6
	testCacheTTL = 10 * time.Second
)

var (
	mockAny = mock.Anything
	mockCtx = context.Background()
)

// ServiceSuite covers Create and Resolve with mocked repo + cache.
type ServiceSuite struct {
	suite.Suite
	repo  *mockRepo
	cache *mockCache
	svc   *Service
}

func (s *ServiceSuite) SetupTest() {
	s.repo = &mockRepo{}
	s.cache = &mockCache{}
	s.svc = New(s.repo, s.cache, testLogger(), testCodeLen, testCacheTTL, nil)
}

func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceSuite))
}

// TestService_Create_NewURL_Success verifies a new long URL is stored and returns a code.
func (s *ServiceSuite) TestService_Create_NewURL_Success() {
	require := s.Require()
	longURL := "https://example.com/a"

	s.repo.On("FindByLongURL", mockCtx, longURL).Return(nil, repository.ErrNotFound).Once()
	s.repo.On("Save", mockCtx, mockAny, longURL).Return(nil).Once()
	code, err := s.svc.Create(context.Background(), longURL)

	require.NoError(err)
	require.Len(code, testCodeLen)
	s.repo.AssertExpectations(s.T())
}

// TestService_Create_ExistingURL_Success verifies an existing long URL reuses its code.
func (s *ServiceSuite) TestService_Create_ExistingURL_Success() {
	require := s.Require()
	longURL := "https://example.com/dup"
	expectedCode := "EXIST1"

	existing := &models.URL{Code: expectedCode, LongURL: longURL}
	s.repo.On("FindByLongURL", mockCtx, longURL).Return(existing, nil).Once()
	code, err := s.svc.Create(context.Background(), longURL)

	require.NoError(err)
	require.Equal(expectedCode, code)
	s.repo.AssertNotCalled(s.T(), "Save")
}

// TestService_Create_FindByLongURLError_Failure verifies a repo error propagates.
func (s *ServiceSuite) TestService_Create_FindByLongURLError_Failure() {
	require := s.Require()
	longURL := "https://example.com/err"
	dbErr := errors.New("db down")

	s.repo.On("FindByLongURL", mockCtx, longURL).Return(nil, dbErr).Once()
	_, err := s.svc.Create(context.Background(), longURL)

	require.ErrorIs(err, dbErr)
}

// TestService_Create_DuplicateCode_Success verifies a code collision retries then succeeds.
func (s *ServiceSuite) TestService_Create_DuplicateCode_Success() {
	require := s.Require()
	longURL := "https://example.com/c"

	s.repo.On("FindByLongURL", mockCtx, longURL).Return(nil, repository.ErrNotFound).Once()
	s.repo.On("Save", mockCtx, mockAny, longURL).Return(repository.ErrDuplicateCode).Once()
	s.repo.On("Save", mockCtx, mockAny, longURL).Return(nil).Once()
	code, err := s.svc.Create(context.Background(), longURL)

	require.NoError(err)
	require.Len(code, testCodeLen)
}

// TestService_Create_SaveError_Failure verifies a non-duplicate save error propagates.
func (s *ServiceSuite) TestService_Create_SaveError_Failure() {
	require := s.Require()
	longURL := "https://example.com/e"
	expectedErr := errors.New("write fail")

	s.repo.On("FindByLongURL", mockCtx, longURL).Return(nil, repository.ErrNotFound).Once()
	s.repo.On("Save", mockCtx, mockAny, longURL).Return(expectedErr).Once()
	_, err := s.svc.Create(context.Background(), longURL)

	require.EqualError(err, expectedErr.Error())
}

// TestService_CreateAsync_Enqueues_Success verifies the pool path enqueues and returns immediately.
func (s *ServiceSuite) TestService_CreateAsync_Enqueues_Success() {
	require := s.Require()
	longURL := "https://example.com/async"

	s.repo.On("FindByLongURL", mockCtx, longURL).Return(nil, repository.ErrNotFound).Once()
	pool := newRecordingPool(s.T())
	svc := New(s.repo, s.cache, testLogger(), testCodeLen, testCacheTTL, pool)
	code, err := svc.Create(context.Background(), longURL)

	require.NoError(err)
	require.Len(code, testCodeLen)
	pool.assertSubmitted(s.T(), 1)
	// Save not called synchronously.
	s.repo.AssertNotCalled(s.T(), "Save")
}

// TestService_CreateAsync_PoolClosed_Failure verifies a closed pool returns ErrPoolClosed.
func (s *ServiceSuite) TestService_CreateAsync_PoolClosed_Failure() {
	require := s.Require()

	s.repo.On("FindByLongURL", mockCtx, mockAny).Return(nil, repository.ErrNotFound).Once()
	pool := newClosedPool()
	svc := New(s.repo, s.cache, testLogger(), testCodeLen, testCacheTTL, pool)
	_, err := svc.Create(context.Background(), "https://example.com/x")

	require.ErrorIs(err, workerErrPoolClosed)
}

// TestService_Resolve_CacheHit_Success verifies a cache hit skips the repo.
func (s *ServiceSuite) TestService_Resolve_CacheHit_Success() {
	require := s.Require()
	expectedLongURL := "https://example.com/hit"

	s.cache.On("Get", mockCtx, "CODE1").Return(expectedLongURL, nil).Once()
	got, err := s.svc.Resolve(context.Background(), "CODE1")

	require.NoError(err)
	require.Equal(expectedLongURL, got)
	s.repo.AssertNotCalled(s.T(), "FindByCode")
}

// TestService_Resolve_CacheMiss_Success verifies a cache miss falls back to repo and backfills.
func (s *ServiceSuite) TestService_Resolve_CacheMiss_Success() {
	require := s.Require()
	expectedLongURL := "https://example.com/miss"

	s.cache.On("Get", mockCtx, "MISS").Return("", cache.ErrCacheMiss).Once()
	s.repo.On("FindByCode", mockCtx, "MISS").
		Return(&models.URL{Code: "MISS", LongURL: expectedLongURL}, nil).Once()
	s.cache.On("Set", mockCtx, "MISS", expectedLongURL, testCacheTTL).Return(nil).Once()
	got, err := s.svc.Resolve(context.Background(), "MISS")

	require.NoError(err)
	require.Equal(expectedLongURL, got)
}

// TestService_Resolve_CacheError_Success verifies a cache error falls back to repo.
func (s *ServiceSuite) TestService_Resolve_CacheError_Success() {
	require := s.Require()
	expectedLongURL := "https://example.com/e"

	s.cache.On("Get", mockCtx, "ERRC").Return("", errors.New("redis down")).Once()
	s.repo.On("FindByCode", mockCtx, "ERRC").
		Return(&models.URL{Code: "ERRC", LongURL: expectedLongURL}, nil).Once()
	s.cache.On("Set", mockCtx, "ERRC", expectedLongURL, testCacheTTL).Return(errors.New("set fail")).Once()
	got, err := s.svc.Resolve(context.Background(), "ERRC")

	require.NoError(err)
	require.Equal(expectedLongURL, got)
}

// TestService_Resolve_NotFound_Failure verifies an unknown code returns ErrNotFound.
func (s *ServiceSuite) TestService_Resolve_NotFound_Failure() {
	require := s.Require()

	s.cache.On("Get", mockCtx, "NF").Return("", cache.ErrCacheMiss).Once()
	s.repo.On("FindByCode", mockCtx, "NF").Return(nil, repository.ErrNotFound).Once()
	_, err := s.svc.Resolve(context.Background(), "NF")

	require.ErrorIs(err, repository.ErrNotFound)
}

// TestService_RandomCode_CorrectLength_Success verifies the generated code length.
func (s *ServiceSuite) TestService_RandomCode_CorrectLength_Success() {
	require := s.Require()

	c, err := randomCode(testCodeLen)

	require.NoError(err)
	require.Len(c, testCodeLen)
}

// TestService_RandomCode_OnlyUnambiguousAlphabet_Success verifies chars come from the unambiguous alphabet.
func (s *ServiceSuite) TestService_RandomCode_OnlyUnambiguousAlphabet_Success() {
	require := s.Require()

	for i := 0; i < 300; i++ {
		c, err := randomCode(testCodeLen)
		require.NoError(err)
		for _, ch := range c {
			require.True(strings.ContainsRune(codeAlphabet, ch), "char %q not in alphabet", ch)
		}
	}
}

// TestService_RandomCode_GeneratesVariedOutput_Success verifies high variety across samples.
func (s *ServiceSuite) TestService_RandomCode_GeneratesVariedOutput_Success() {
	require := s.Require()
	expectedMinUnique := 50

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		c, err := randomCode(testCodeLen)
		require.NoError(err)
		seen[c] = true
	}

	// 6-char codes from a 56-char alphabet: expecting high variety.
	require.Greater(len(seen), expectedMinUnique, "expected varied codes, got %d unique", len(seen))
}

// TestService_New_NilCache_Success verifies a nil cache defaults to Noop without panicking.
func (s *ServiceSuite) TestService_New_NilCache_Success() {
	require := s.Require()
	expectedCodeLen := 6

	s.repo.On("FindByLongURL", mockCtx, mockAny).Return(nil, repository.ErrNotFound).Once()
	s.repo.On("Save", mockCtx, mockAny, mockAny).Return(nil).Once()
	svc := New(s.repo, nil, testLogger(), 0, testCacheTTL, nil) // codeLen 0 -> default 6
	code, err := svc.Create(context.Background(), "https://example.com/n")

	require.NoError(err)
	require.Len(code, expectedCodeLen)
}

// TestService_Create_AllAttemptsCollide_Failure verifies exhausted collisions return ErrCodeGeneration.
func (s *ServiceSuite) TestService_Create_AllAttemptsCollide_Failure() {
	require := s.Require()

	s.repo.On("FindByLongURL", mockCtx, mockAny).Return(nil, repository.ErrNotFound).Once()
	s.repo.On("Save", mockCtx, mockAny, mockAny).Return(repository.ErrDuplicateCode)
	_, err := s.svc.Create(context.Background(), "https://example.com/exhaust")

	require.ErrorIs(err, ErrCodeGeneration)
}
