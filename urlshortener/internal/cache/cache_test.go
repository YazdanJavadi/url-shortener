package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	redismock "github.com/go-redis/redismock/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

func testLogger() *logrus.Entry {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)

	return logrus.NewEntry(l)
}

// CacheSuite exercises the Redis cache impl against a redismock client.
type CacheSuite struct {
	suite.Suite
	mock redismock.ClientMock
	c    *Redis
}

func (s *CacheSuite) SetupTest() {
	client, mock := redismock.NewClientMock()
	s.mock = mock
	s.c = NewRedis(client, testLogger())
}

func TestCacheSuite(t *testing.T) {
	suite.Run(t, new(CacheSuite))
}

// TestCache_Get_Hit_Success verifies a cache hit returns the stored value.
func (s *CacheSuite) TestCache_Get_Hit_Success() {
	require := s.Require()
	expectedVal := "v"

	s.mock.ExpectGet("k").SetVal("v")
	got, err := s.c.Get(context.Background(), "k")

	require.NoError(err)
	require.Equal(expectedVal, got)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Get_Miss_Failure verifies a redis nil maps to ErrCacheMiss.
func (s *CacheSuite) TestCache_Get_Miss_Failure() {
	require := s.Require()

	s.mock.ExpectGet("k").RedisNil()
	_, err := s.c.Get(context.Background(), "k")

	require.ErrorIs(err, ErrCacheMiss)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Get_Error_Failure verifies a redis error propagates.
func (s *CacheSuite) TestCache_Get_Error_Failure() {
	require := s.Require()
	expectedErr := errors.New("conn reset")

	s.mock.ExpectGet("k").SetErr(expectedErr)
	_, err := s.c.Get(context.Background(), "k")

	require.EqualError(err, expectedErr.Error())
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Set_OK_Success verifies a successful set.
func (s *CacheSuite) TestCache_Set_OK_Success() {
	require := s.Require()
	ttl := 5 * time.Second

	s.mock.ExpectSet("k", "v", ttl).SetVal("OK")
	err := s.c.Set(context.Background(), "k", "v", ttl)

	require.NoError(err)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Set_Error_Failure verifies a set error propagates.
func (s *CacheSuite) TestCache_Set_Error_Failure() {
	require := s.Require()
	ttl := 5 * time.Second

	s.mock.ExpectSet("k", "v", ttl).SetErr(errors.New("oom"))
	err := s.c.Set(context.Background(), "k", "v", ttl)

	require.Error(err)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Del_OK_Success verifies a successful delete.
func (s *CacheSuite) TestCache_Del_OK_Success() {
	require := s.Require()

	s.mock.ExpectDel("k").SetVal(1)
	err := s.c.Del(context.Background(), "k")

	require.NoError(err)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Del_Error_Failure verifies a delete error propagates.
func (s *CacheSuite) TestCache_Del_Error_Failure() {
	require := s.Require()

	s.mock.ExpectDel("k").SetErr(errors.New("err"))
	err := s.c.Del(context.Background(), "k")

	require.Error(err)
	require.NoError(s.mock.ExpectationsWereMet())
}

// TestCache_Noop_AlwaysMiss_Success verifies the Noop cache contract.
func (s *CacheSuite) TestCache_Noop_AlwaysMiss_Success() {
	require := s.Require()

	n := Noop{}
	_, err := n.Get(context.Background(), "x")

	require.ErrorIs(err, ErrCacheMiss)
	require.NoError(n.Set(context.Background(), "x", "y", time.Second))
	require.NoError(n.Del(context.Background(), "x"))
}
