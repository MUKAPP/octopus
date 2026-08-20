package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestStaticCacheControlByPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fileSystem := http.FS(fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("index")},
		"assets/app.js": &fstest.MapFile{Data: []byte("asset")},
	})
	handler := static("", fileSystem)

	tests := []struct {
		name       string
		path       string
		wantHeader string
	}{
		{
			name:       "hashed asset is immutable",
			path:       "/assets/app.js",
			wantHeader: "public, max-age=31536000, immutable",
		},
		{
			name:       "app shell is revalidated",
			path:       "/index.html",
			wantHeader: "no-cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)

			handler(context)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Cache-Control"); got != tt.wantHeader {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.wantHeader)
			}
		})
	}
}
