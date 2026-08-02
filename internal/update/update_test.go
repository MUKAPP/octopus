package update

import (
	"fmt"
	"testing"
)

func TestGetLatestInfoReturnsVersionForLatestImage(t *testing.T) {
	const latestTagURL = "https://hub.test/tags/latest"
	const tagsURL = "https://hub.test/tags?page_size=100"

	info, err := getLatestInfo(func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-02T10:30:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"latest","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]},{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]},{"name":"dev-deadbee","images":[{"digest":"sha256:other"}]}]}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL)
	if err != nil {
		t.Fatalf("获取最新镜像版本失败：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("镜像版本不符合预期：got %q, want %q", info.TagName, "dev-4a8f7da")
	}
	if info.PublishedAt != "2026-08-02T10:30:00Z" {
		t.Fatalf("镜像发布时间不符合预期：%q", info.PublishedAt)
	}
}

func TestGetLatestInfoRejectsTagWithDifferentImage(t *testing.T) {
	const latestTagURL = "https://hub.test/tags/latest"
	const tagsURL = "https://hub.test/tags?page_size=100"

	_, err := getLatestInfo(func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","images":[{"digest":"sha256:current"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"dev-deadbee","images":[{"digest":"sha256:older"}]}]}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL)
	if err == nil {
		t.Fatal("开发镜像与 latest 不同时应返回错误")
	}
}

func TestImageVersion(t *testing.T) {
	tests := []struct {
		tagName string
		want    string
		ok      bool
	}{
		{tagName: "dev-4a8f7da", want: "dev-4a8f7da", ok: true},
		{tagName: "sha-4a8f7da", want: "dev-4a8f7da", ok: true},
		{tagName: "latest", ok: false},
	}
	for _, tt := range tests {
		got, ok := imageVersion(tt.tagName)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("imageVersion(%q) = (%q, %t), want (%q, %t)", tt.tagName, got, ok, tt.want, tt.ok)
		}
	}
}
