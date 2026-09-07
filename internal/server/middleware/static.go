package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/gin-gonic/gin"
)

// StaticEmbed 返回用于嵌入式静态文件系统的 HTTP 中间件。
func StaticEmbed(urlPrefix string, embedFS fs.FS) gin.HandlerFunc {
	return static(urlPrefix, http.FS(embedFS))
}

// StaticLocal 返回用于本地静态文件目录的 HTTP 中间件。
func StaticLocal(urlPrefix string, localPath string) gin.HandlerFunc {
	return static(urlPrefix, http.Dir(localPath))
}

func openStatic(fileSystem http.FileSystem, name string) http.File {
	file, err := fileSystem.Open(name)
	if err != nil {
		return nil
	}
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		file.Close()
		return nil
	}
	return file
}

func parseQuality(value string) float64 {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	quality, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(quality) || quality < 0 || quality > 1 {
		return 0
	}
	return quality
}

func parseAcceptEncoding(header string) (gzipQuality, identityQuality float64) {
	identityQuality = 1
	wildcardQuality := -1.0
	var gzipSet, identitySet, wildcardSet bool

	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		encoding := strings.TrimSpace(parts[0])
		if encoding == "" {
			continue
		}

		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(parameter, "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				quality = parseQuality(value)
				break
			}
		}

		switch {
		case strings.EqualFold(encoding, "gzip"):
			gzipQuality = quality
			gzipSet = true
		case strings.EqualFold(encoding, "identity"):
			identityQuality = quality
			identitySet = true
		case encoding == "*":
			wildcardQuality = quality
			wildcardSet = true
		}
	}

	if !gzipSet && wildcardSet {
		gzipQuality = wildcardQuality
	}
	if !identitySet && wildcardSet && wildcardQuality == 0 {
		identityQuality = 0
	}
	return gzipQuality, identityQuality
}

func selectStatic(fileSystem http.FileSystem, name string, gzipQuality, identityQuality float64) (http.File, bool, bool) {
	preferGzip := gzipQuality > identityQuality || (gzipQuality == identityQuality && gzipQuality > 0)
	if preferGzip {
		if file := openStatic(fileSystem, name+".gz"); file != nil {
			return file, true, false
		}
	}
	if identityQuality > 0 {
		if file := openStatic(fileSystem, name); file != nil {
			return file, false, false
		}
		if file := openStatic(fileSystem, name+".gz"); file != nil {
			return file, false, true
		}
	}
	if !preferGzip && gzipQuality > 0 {
		if file := openStatic(fileSystem, name+".gz"); file != nil {
			return file, true, false
		}
	}
	return nil, false, false
}

func hasStatic(fileSystem http.FileSystem, name string) bool {
	if file := openStatic(fileSystem, name); file != nil {
		file.Close()
		return true
	}
	if file := openStatic(fileSystem, name+".gz"); file != nil {
		file.Close()
		return true
	}
	return false
}

// static 根据文件类型设置缓存策略并响应已存在的静态文件。
func static(urlPrefix string, fileSystem http.FileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Next()
			return
		}
		name := path.Clean("/" + strings.TrimPrefix(c.Request.URL.Path, urlPrefix))
		if name == "/" {
			name = "/index.html"
		}

		gzipQuality, identityQuality := parseAcceptEncoding(c.GetHeader("Accept-Encoding"))
		file, encoded, inflate := selectStatic(fileSystem, name, gzipQuality, identityQuality)
		if file == nil {
			if hasStatic(fileSystem, name) {
				c.Header("Vary", "Accept-Encoding")
				resp.Error(c, http.StatusNotAcceptable, http.StatusText(http.StatusNotAcceptable))
				return
			}
			c.Next()
			return
		}
		defer file.Close()

		if strings.HasPrefix(name, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		c.Header("Vary", "Accept-Encoding")

		var content io.ReadSeeker = file
		switch {
		case encoded:
			c.Header("Content-Encoding", "gzip")
			if ctype := mime.TypeByExtension(path.Ext(name)); ctype != "" {
				c.Header("Content-Type", ctype)
			}
		case inflate:
			reader, err := gzip.NewReader(file)
			if err != nil {
				resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
				c.Abort()
				return
			}
			defer reader.Close()
			raw, err := io.ReadAll(reader)
			if err != nil {
				resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
				c.Abort()
				return
			}
			content = bytes.NewReader(raw)
		}
		http.ServeContent(c.Writer, c.Request, name, time.Time{}, content)
		c.Abort()
	}
}
