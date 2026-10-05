package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/yazdanjavadi/urlshort/internal/clock"
	"github.com/yazdanjavadi/urlshort/internal/models"
)

func testLogger() *logrus.Entry {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)

	return logrus.NewEntry(l)
}

// fixedTime is the deterministic time injected into the repo for these tests,
// so the dynamic CreatedAt/UpdatedAt values are predictable and assertable.
var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// RepoSuite tests the GORM repository against a go-sqlmock DB.
type RepoSuite struct {
	suite.Suite
	db   *sql.DB
	mock sqlmock.Sqlmock
	repo Repository
}

func (s *RepoSuite) SetupTest() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	s.db = db
	s.mock = mock

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{SkipDefaultTransaction: true})
	s.Require().NoError(err)

	s.repo = New(gormDB, clock.Fixed(fixedTime))
}

func (s *RepoSuite) TearDownTest() {
	s.mock.MatchExpectationsInOrder(false)
	s.NoError(s.mock.ExpectationsWereMet())
	s.mock.ExpectClose()
	s.NoError(s.db.Close())
}

func TestRepoSuite(t *testing.T) {
	suite.Run(t, new(RepoSuite))
}

// TestRepo_Save_InsertsWithClockTimestamps_Success verifies Save stamps clock timestamps.
func (s *RepoSuite) TestRepo_Save_InsertsWithClockTimestamps_Success() {
	require := s.Require()
	code := "CODE1"
	longURL := "https://example.com"

	// GORM postgres INSERT uses RETURNING. Match the columns it selects back.
	s.mock.ExpectQuery(`INSERT INTO "urls" (.+) VALUES`).
		WithArgs(code, longURL, fixedTime, fixedTime).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	err := s.repo.Save(context.Background(), code, longURL)

	require.NoError(err)
}

// TestRepo_Save_DuplicateKey_Failure verifies a unique violation maps to ErrDuplicateCode.
func (s *RepoSuite) TestRepo_Save_DuplicateKey_Failure() {
	require := s.Require()
	code := "DUP"
	longURL := "https://example.com"

	s.mock.ExpectQuery(`INSERT INTO "urls"`).
		WithArgs(code, longURL, fixedTime, fixedTime).
		WillReturnError(errors.New(`pq: duplicate key value violates unique constraint "idx_urls_code" (SQLSTATE 23505)`))
	err := s.repo.Save(context.Background(), code, longURL)

	require.ErrorIs(err, ErrDuplicateCode)
}

// TestRepo_Save_GenericError_Failure verifies a generic error propagates.
func (s *RepoSuite) TestRepo_Save_GenericError_Failure() {
	require := s.Require()
	code := "E"
	longURL := "https://example.com"
	expectedErr := errors.New("connection refused")

	s.mock.ExpectQuery(`INSERT INTO "urls"`).
		WithArgs(code, longURL, fixedTime, fixedTime).
		WillReturnError(expectedErr)
	err := s.repo.Save(context.Background(), code, longURL)

	require.EqualError(err, expectedErr.Error())
}

// TestRepo_FindByCode_Found_Success verifies FindByCode loads the record.
func (s *RepoSuite) TestRepo_FindByCode_Found_Success() {
	require := s.Require()
	code := "F1"
	expectedLongURL := "https://example.com/f"

	rows := sqlmock.NewRows([]string{"id", "code", "long_url", "created_at", "updated_at"}).
		AddRow(1, code, expectedLongURL, fixedTime, fixedTime)
	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(code, sqlmock.AnyArg()).
		WillReturnRows(rows)
	rec, err := s.repo.FindByCode(context.Background(), code)

	require.NoError(err)
	require.Equal(code, rec.Code)
	require.Equal(expectedLongURL, rec.LongURL)
	require.Equal(fixedTime, rec.CreatedAt)
}

// TestRepo_FindByCode_NotFound_Failure verifies a missing record maps to ErrNotFound.
func (s *RepoSuite) TestRepo_FindByCode_NotFound_Failure() {
	require := s.Require()
	code := "NF"

	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(code, sqlmock.AnyArg()).
		WillReturnError(gorm.ErrRecordNotFound)
	_, err := s.repo.FindByCode(context.Background(), code)

	require.ErrorIs(err, ErrNotFound)
}

// TestRepo_FindByCode_GenericError_Failure verifies a generic error propagates.
func (s *RepoSuite) TestRepo_FindByCode_GenericError_Failure() {
	require := s.Require()
	code := "E"
	expectedErr := errors.New("boom")

	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(code, sqlmock.AnyArg()).
		WillReturnError(expectedErr)
	_, err := s.repo.FindByCode(context.Background(), code)

	require.EqualError(err, expectedErr.Error())
}

// TestRepo_FindByLongURL_Found_Success verifies FindByLongURL loads the record.
func (s *RepoSuite) TestRepo_FindByLongURL_Found_Success() {
	require := s.Require()
	longURL := "https://example.com/l"
	expectedCode := "L1"

	rows := sqlmock.NewRows([]string{"id", "code", "long_url", "created_at", "updated_at"}).
		AddRow(2, expectedCode, longURL, fixedTime, fixedTime)
	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(longURL, sqlmock.AnyArg()).
		WillReturnRows(rows)
	rec, err := s.repo.FindByLongURL(context.Background(), longURL)

	require.NoError(err)
	require.Equal(expectedCode, rec.Code)
}

// TestRepo_FindByLongURL_NotFound_Failure verifies a missing record maps to ErrNotFound.
func (s *RepoSuite) TestRepo_FindByLongURL_NotFound_Failure() {
	require := s.Require()
	longURL := "https://example.com/x"

	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(longURL, sqlmock.AnyArg()).
		WillReturnError(gorm.ErrRecordNotFound)
	_, err := s.repo.FindByLongURL(context.Background(), longURL)

	require.ErrorIs(err, ErrNotFound)
}

// TestRepo_FindByLongURL_GenericError_Failure verifies a generic error propagates.
func (s *RepoSuite) TestRepo_FindByLongURL_GenericError_Failure() {
	require := s.Require()
	longURL := "https://example.com/x"
	expectedErr := errors.New("boom")

	s.mock.ExpectQuery(`SELECT (.+) FROM "urls"`).
		WithArgs(longURL, sqlmock.AnyArg()).
		WillReturnError(expectedErr)
	_, err := s.repo.FindByLongURL(context.Background(), longURL)

	require.EqualError(err, expectedErr.Error())
}

// TestRepo_IsDuplicateKeyError_Success verifies the SQLSTATE 23505 matcher.
func (s *RepoSuite) TestRepo_IsDuplicateKeyError_Success() {
	require := s.Require()

	require.True(isDuplicateKeyError(errors.New("SQLSTATE 23505")))
	require.False(isDuplicateKeyError(errors.New("other")))
	require.False(isDuplicateKeyError(nil))
}

// keep regexp import used.
var _ = regexp.MustCompile

// keep models import used.
var _ = models.URL{}
