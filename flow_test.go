package flow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type routeTest struct {
	name                string
	routeMethods        []string
	routePattern        string
	requestMethod       string
	requestPath         string
	expectedStatus      int
	expectedParams      map[string]string
	expectedAllowHeader string
	expectedBody        string
}

func TestRouter(t *testing.T) {
	t.Run("Route Matching", testRouteMatching)
	t.Run("Middleware Chain", testMiddlewareChain)
	t.Run("Custom Handlers", testCustomHandlers)
	t.Run("URL Parameters", testURLParameters)
}

func testRouteMatching(t *testing.T) {
	tests := []routeTest{
		{
			name:           "Simple Path Match",
			routeMethods:   []string{"GET"},
			routePattern:   "/one",
			requestMethod:  "GET",
			requestPath:    "/one",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Wildcard Match",
			routeMethods:   []string{"GET"},
			routePattern:   "/prefix/...",
			requestMethod:  "GET",
			requestPath:    "/prefix/anything/else",
			expectedStatus: http.StatusOK,
			expectedParams: map[string]string{"...": "anything/else"},
		},
		{
			name:           "Path Parameters Match",
			routeMethods:   []string{"GET"},
			routePattern:   "/path-params/:era/:group/:member",
			requestMethod:  "GET",
			requestPath:    "/path-params/60/beatles/lennon",
			expectedStatus: http.StatusOK,
			expectedParams: map[string]string{
				"era":    "60",
				"group":  "beatles",
				"member": "lennon",
			},
		},
		{
			name:           "Regexp Pattern Match",
			routeMethods:   []string{"GET"},
			routePattern:   "/path-params/:era|^[0-9]{2}$/:group|^[a-z].+$",
			requestMethod:  "GET",
			requestPath:    "/path-params/60/beatles",
			expectedStatus: http.StatusOK,
			expectedParams: map[string]string{
				"era":   "60",
				"group": "beatles",
			},
		},
		// Add more test cases here...
	}

	runRouteTests(t, tests)
}

func testMiddlewareChain(t *testing.T) {
	middlewareOrder := ""

	createMiddleware := func(id string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareOrder += id
				next.ServeHTTP(w, r)
			})
		}
	}

	tests := []struct {
		name           string
		path           string
		method         string
		setupMux       func(*Mux)
		expectedOrder  string
		expectedStatus int
	}{
		{
			name:   "Base Route Middleware",
			path:   "/",
			method: "GET",
			setupMux: func(m *Mux) {
				m.Use(createMiddleware("1"), createMiddleware("2"))
				m.HandleFunc("/", emptyHandler, "GET")
			},
			expectedOrder:  "12",
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Nested Group Middleware",
			path:   "/nested/foo",
			method: "GET",
			setupMux: func(m *Mux) {
				m.Use(createMiddleware("1"))
				m.Group(func(m *Mux) {
					m.Use(createMiddleware("2"))
					m.Group(func(m *Mux) {
						m.Use(createMiddleware("3"))
						m.HandleFunc("/nested/foo", emptyHandler, "GET")
					})
				})
			},
			expectedOrder:  "123",
			expectedStatus: http.StatusOK,
		},
		// Add more middleware test cases...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middlewareOrder = ""
			mux := New()
			tt.setupMux(mux)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, rec.Code)
			}
			if middlewareOrder != tt.expectedOrder {
				t.Errorf("expected middleware order %q; got %q", tt.expectedOrder, middlewareOrder)
			}
		})
	}
}

func testCustomHandlers(t *testing.T) {
	tests := []routeTest{
		{
			name:           "Custom Not Found Handler",
			routeMethods:   []string{"GET"},
			routePattern:   "/",
			requestMethod:  "GET",
			requestPath:    "/notfound",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "custom not found handler",
		},
		{
			name:           "Custom Method Not Allowed Handler",
			routeMethods:   []string{"GET"},
			routePattern:   "/",
			requestMethod:  "POST",
			requestPath:    "/",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "custom method not allowed handler",
		},
		// Add more custom handler tests...
	}

	mux := New()
	mux.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("custom not found handler"))
	})
	mux.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("custom method not allowed handler"))
	})

	mux.HandleFunc("/", emptyHandler, "GET")
	runCustomHandlerTests(t, mux, tests)
}

func testURLParameters(t *testing.T) {
	tests := []struct {
		name          string
		pattern       string
		path          string
		paramName     string
		expectedValue string
		shouldExist   bool
	}{
		{
			name:          "Simple Parameter",
			pattern:       "/users/:id",
			path:          "/users/123",
			paramName:     "id",
			expectedValue: "123",
			shouldExist:   true,
		},
		{
			name:          "Missing Parameter",
			pattern:       "/users/:id",
			path:          "/users/123",
			paramName:     "unknown",
			expectedValue: "",
			shouldExist:   false,
		},
		// Add more parameter test cases...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			mux := New()
			mux.HandleFunc(tt.pattern, func(w http.ResponseWriter, r *http.Request) {
				ctx = r.Context()
			}, "GET")

			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			value := Param(ctx, tt.paramName)
			if value != tt.expectedValue {
				t.Errorf("expected parameter value %q; got %q", tt.expectedValue, value)
			}
		})
	}
}

// Helper functions
func runRouteTests(t *testing.T, tests []routeTest) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			mux := New()

			mux.HandleFunc(tt.routePattern, func(w http.ResponseWriter, r *http.Request) {
				ctx = r.Context()
			}, tt.routeMethods...)

			req := httptest.NewRequest(tt.requestMethod, tt.requestPath, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, rec.Code)
			}

			if len(tt.expectedParams) > 0 {
				for param, expected := range tt.expectedParams {
					if actual := Param(ctx, param); actual != expected {
						t.Errorf("parameter %q: expected %q; got %q", param, expected, actual)
					}
				}
			}

			if tt.expectedAllowHeader != "" {
				if actual := rec.Header().Get("Allow"); actual != tt.expectedAllowHeader {
					t.Errorf("expected Allow header %q; got %q", tt.expectedAllowHeader, actual)
				}
			}
		})
	}
}

func runCustomHandlerTests(t *testing.T, mux *Mux, tests []routeTest) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.requestMethod, tt.requestPath, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, rec.Code)
			}

			if string(body) != tt.expectedBody {
				t.Errorf("expected body %q; got %q", tt.expectedBody, string(body))
			}
		})
	}
}

func emptyHandler(w http.ResponseWriter, r *http.Request) {}
