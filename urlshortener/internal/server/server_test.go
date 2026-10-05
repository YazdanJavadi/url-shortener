package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/yazdanjavadi/urlshort/internal/repository"
	"github.com/yazdanjavadi/urlshort/internal/urlservice"
)

func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)

	return l
}

// mockService satisfies server.URLService for controller tests.
type mockService struct{ mock.Mock }

func (m *mockService) Create(ctx context.Context, longURL string) (string, error) {
	args := m.Called(ctx, longURL)

	return args.String(0), args.Error(1)
}

func (m *mockService) Resolve(ctx context.Context, code string) (string, error) {
	args := m.Called(ctx, code)

	return args.String(0), args.Error(1)
}

type ServerSuite struct {
	suite.Suite
	svc *mockService
	srv *Server
}

func (s *ServerSuite) SetupTest() {
	s.svc = &mockService{}
	s.srv = New(s.svc, testLogger(), "http://localhost:8080")
}

func TestServerSuite(t *testing.T) { suite.Run(t, new(ServerSuite)) }

// TestServer_Create_Success verifies a valid POST /urls returns the short URL.
func (s *ServerSuite) TestServer_Create_Success() {
	require := s.Require()
	longURL := "https://example.com/a"
	expectedCode := "CODE1"
	expectedShort := "http://localhost:8080/CODE1"

	s.svc.On("Create", mock.Anything, longURL).Return(expectedCode, nil).Once()
	c, rec := s.request(http.MethodPost, "/urls", `{"url":"https://example.com/a"}`, nil)
	err := s.srv.createShortURL(c)

	require.NoError(err)
	require.Equal(http.StatusCreated, rec.Code)
	var body map[string]string
	require.NoError(json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(expectedCode, body["code"])
	require.Equal(expectedShort, body["short"])
	require.Equal(longURL, body["long_url"])
}

// TestServer_Create_InvalidJSON_Failure verifies invalid JSON yields a bad-request error.
func (s *ServerSuite) TestServer_Create_InvalidJSON_Failure() {
	require := s.Require()

	c, _ := s.request(http.MethodPost, "/urls", `not json`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusBadRequest)
	require.NotNil(err)
}

// TestServer_Create_EmptyURL_Failure verifies an empty URL yields a bad-request error.
func (s *ServerSuite) TestServer_Create_EmptyURL_Failure() {
	c, _ := s.request(http.MethodPost, "/urls", `{"url":""}`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusBadRequest)
}

// TestServer_Create_BadScheme_Failure verifies a non-http scheme yields a bad-request error.
func (s *ServerSuite) TestServer_Create_BadScheme_Failure() {
	c, _ := s.request(http.MethodPost, "/urls", `{"url":"ftp://x"}`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusBadRequest)
}

// TestServer_Create_NoHost_Failure verifies a hostless URL yields a bad-request error.
func (s *ServerSuite) TestServer_Create_NoHost_Failure() {
	c, _ := s.request(http.MethodPost, "/urls", `{"url":"http:///path"}`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusBadRequest)
}

// TestServer_Create_CodeGenFailure_Failure verifies a code-gen error yields an internal error.
func (s *ServerSuite) TestServer_Create_CodeGenFailure_Failure() {
	s.svc.On("Create", mock.Anything, "https://example.com/e").Return("", urlservice.ErrCodeGeneration).Once()

	c, _ := s.request(http.MethodPost, "/urls", `{"url":"https://example.com/e"}`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusInternalServerError)
}

// TestServer_Create_ServiceError_Failure verifies a service error yields an internal error.
func (s *ServerSuite) TestServer_Create_ServiceError_Failure() {
	s.svc.On("Create", mock.Anything, "https://example.com/e2").Return("", errors.New("db down")).Once()

	c, _ := s.request(http.MethodPost, "/urls", `{"url":"https://example.com/e2"}`, nil)
	err := s.srv.createShortURL(c)

	s.assertHTTPError(err, http.StatusInternalServerError)
}

// TestServer_Resolve_Success verifies GET /:code redirects to the long URL.
func (s *ServerSuite) TestServer_Resolve_Success() {
	require := s.Require()
	expectedLongURL := "https://example.com/dest"

	s.svc.On("Resolve", mock.Anything, "CODE2").Return(expectedLongURL, nil).Once()
	c, rec := s.request(http.MethodGet, "/CODE2", "", nil)
	c.SetParamNames("code")
	c.SetParamValues("CODE2")
	err := s.srv.resolveShortURL(c)

	require.NoError(err)
	require.Equal(http.StatusFound, rec.Code)
	require.Equal(expectedLongURL, rec.Header().Get(echo.HeaderLocation))
}

// TestServer_Resolve_NotFound_Failure verifies an unknown code yields a not-found error.
func (s *ServerSuite) TestServer_Resolve_NotFound_Failure() {
	s.svc.On("Resolve", mock.Anything, "NF").Return("", repository.ErrNotFound).Once()

	c, _ := s.request(http.MethodGet, "/NF", "", nil)
	c.SetParamNames("code")
	c.SetParamValues("NF")
	err := s.srv.resolveShortURL(c)

	s.assertHTTPError(err, http.StatusNotFound)
}

// TestServer_Resolve_ServiceError_Failure verifies a service error yields an internal error.
func (s *ServerSuite) TestServer_Resolve_ServiceError_Failure() {
	s.svc.On("Resolve", mock.Anything, "E").Return("", errors.New("boom")).Once()

	c, _ := s.request(http.MethodGet, "/E", "", nil)
	c.SetParamNames("code")
	c.SetParamValues("E")
	err := s.srv.resolveShortURL(c)

	s.assertHTTPError(err, http.StatusInternalServerError)
}

// TestServer_Resolve_EmptyCode_Failure verifies an empty code yields a bad-request error.
func (s *ServerSuite) TestServer_Resolve_EmptyCode_Failure() {
	c, _ := s.request(http.MethodGet, "/", "", nil)
	c.SetParamNames("code")
	c.SetParamValues("")
	err := s.srv.resolveShortURL(c)

	s.assertHTTPError(err, http.StatusBadRequest)
}

// TestServer_ShortURL_UsesBaseURL_Success verifies shortURL uses the configured base URL.
func (s *ServerSuite) TestServer_ShortURL_UsesBaseURL_Success() {
	require := s.Require()
	expectedShort := "http://localhost:8080/ABC"

	c, _ := s.request(http.MethodGet, "/x", "", nil)
	got := s.srv.shortURL(c, "ABC")

	require.Equal(expectedShort, got)
}

// TestServer_ShortURL_FallsBackToRequestHost_Success verifies shortURL falls back to the request host.
func (s *ServerSuite) TestServer_ShortURL_FallsBackToRequestHost_Success() {
	require := s.Require()
	expectedShort := "http://example.com/ABC"

	srv := New(s.svc, testLogger(), "")
	c, _ := s.request(http.MethodGet, "/x", "", nil)
	got := srv.shortURL(c, "ABC")

	require.Equal(expectedShort, got)
}

// TestServer_ValidateURL_Success verifies the URL validator accepts and rejects correctly.
func (s *ServerSuite) TestServer_ValidateURL_Success() {
	require := s.Require()

	require.NoError(validateURL("http://example.com"))
	require.NoError(validateURL("https://example.com/path"))
	require.Error(validateURL("ftp://example.com"))
	require.Error(validateURL("http:///nohost"))
	require.Error(validateURL("not a url"))
}

// TestServer_ValidateURL_ParseError_Failure verifies a control character makes url.Parse fail.
func (s *ServerSuite) TestServer_ValidateURL_ParseError_Failure() {
	require := s.Require()

	require.Error(validateURL("http://example.com/\x7f"))
}

// TestServer_ShortURL_TLSScheme_Success verifies shortURL uses https under TLS.
func (s *ServerSuite) TestServer_ShortURL_TLSScheme_Success() {
	require := s.Require()
	expectedShort := "https://example.com/ABC"

	srv := New(s.svc, testLogger(), "")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{} // non-nil => c.IsTLS() true
	rec := httptest.NewRecorder()
	c := srv.echo.NewContext(req, rec)
	got := srv.shortURL(c, "ABC")

	require.Equal(expectedShort, got)
}

// TestServer_Start_Method_BindsAndServes_Success verifies Start binds a real socket and serves.
func (s *ServerSuite) TestServer_Start_Method_BindsAndServes_Success() {
	require := s.Require()
	expectedStatus := http.StatusFound

	svc := &mockService{}
	svc.On("Resolve", mock.Anything, "ping").Return("https://example.com/ping", nil)
	srv := New(svc, testLogger(), "http://localhost:8080")

	addr := freePortAddr(s.T())
	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(addr) }()

	url := "http://" + addr + "/ping"
	client := &http.Client{
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		resp, err = client.Get(url)
		if err == nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.NoError(err)
	require.Equal(expectedStatus, resp.StatusCode)
	resp.Body.Close()
	require.NoError(srv.echo.Close())
	select {
	case <-startErr:
	case <-time.After(2 * time.Second):
		s.Fail("Start did not return after Close")
	}
}

// TestServer_Start_BindsAndServes_Success verifies echo.Start binds a real socket and serves.
func (s *ServerSuite) TestServer_Start_BindsAndServes_Success() {
	require := s.Require()
	expectedStatus := http.StatusFound

	svc := &mockService{}
	svc.On("Resolve", mock.Anything, "ping").Return("https://example.com/ping", nil)
	srv := New(svc, testLogger(), "http://localhost:8080")

	// Find a free port, release it, then let echo bind it.
	addr := freePortAddr(s.T())
	startErr := make(chan error, 1)
	go func() { startErr <- srv.echo.Start(addr) }()

	url := "http://" + addr + "/ping"
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		resp, err = client.Get(url)
		if err == nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.NoError(err, "server did not respond in time")
	require.Equal(expectedStatus, resp.StatusCode)
	resp.Body.Close()

	// Shut the server down so the Start goroutine returns and the test exits.
	require.NoError(srv.echo.Close())
	select {
	case <-startErr:
	case <-time.After(2 * time.Second):
		s.Fail("echo.Start did not return after Close")
	}
}

// request builds an Echo context + recorder for a handler under test.
func (s *ServerSuite) request(
	method, path, body string,
	_ map[string]string,
) (echo.Context, *httptest.ResponseRecorder) {
	var r *strings.Reader
	if body != "" {
		r = strings.NewReader(body)
	} else {
		r = strings.NewReader("")
	}

	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}

	rec := httptest.NewRecorder()
	c := s.srv.echo.NewContext(req, rec)

	return c, rec
}

// assertHTTPError verifies err is an echo.HTTPError with the given status code.
func (s *ServerSuite) assertHTTPError(err error, code int) {
	require := s.Require()

	require.Error(err)
	he, ok := err.(*echo.HTTPError)
	require.True(ok, "expected echo.HTTPError, got %T", err)
	require.Equal(code, he.Code)
}

// TestServer_Itoa_Success verifies the status-code to string converter.
func (s *ServerSuite) TestServer_Itoa_Success() {
	require := s.Require()

	require.Equal("200", itoa(200))
	require.Equal("302", itoa(302))
	require.Equal("404", itoa(404))
	require.Equal("0", itoa(0))
}
