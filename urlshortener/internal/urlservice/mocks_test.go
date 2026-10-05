package urlservice

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/yazdanjavadi/urlshort/internal/models"
)

// mockRepo is a testify/mock Repository for unit-testing the service.
type mockRepo struct{ mock.Mock }

func (m *mockRepo) Save(ctx context.Context, code, longURL string) error {
	return m.Called(ctx, code, longURL).Error(0)
}

func (m *mockRepo) FindByCode(ctx context.Context, code string) (*models.URL, error) {
	args := m.Called(ctx, code)
	var rec *models.URL
	if args.Get(0) != nil {
		rec = args.Get(0).(*models.URL)
	}

	return rec, args.Error(1)
}

func (m *mockRepo) FindByLongURL(ctx context.Context, longURL string) (*models.URL, error) {
	args := m.Called(ctx, longURL)
	var rec *models.URL
	if args.Get(0) != nil {
		rec = args.Get(0).(*models.URL)
	}

	return rec, args.Error(1)
}

// mockCache is a testify/mock Cache for unit-testing the service.
type mockCache struct{ mock.Mock }

func (m *mockCache) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)

	return args.String(0), args.Error(1)
}

func (m *mockCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return m.Called(ctx, key, value, ttl).Error(0)
}

func (m *mockCache) Del(ctx context.Context, key string) error {
	return m.Called(ctx, key).Error(0)
}
