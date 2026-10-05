package models

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ModelSuite struct{ suite.Suite }

func TestModelSuite(t *testing.T) { suite.Run(t, new(ModelSuite)) }

// TestModel_URL_TableName_Success verifies the GORM table name override.
func (s *ModelSuite) TestModel_URL_TableName_Success() {
	require := s.Require()
	expectedTableName := "urls"

	got := (URL{}).TableName()

	require.Equal(expectedTableName, got)
}

// TestModel_URL_ZeroValue_Success verifies a zero-value URL is empty.
func (s *ModelSuite) TestModel_URL_ZeroValue_Success() {
	require := s.Require()

	u := URL{}

	require.Empty(u.Code)
	require.Empty(u.LongURL)
	require.Zero(u.ID)
	require.True(u.CreatedAt.IsZero())
	require.True(u.UpdatedAt.IsZero())
}
