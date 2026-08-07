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

func TestGetLatestReleaseInfoReturnsVersion(t *testing.T) {
	const releaseURL = "https://api.test/releases/tags/octopus-binaries"

	info, err := getLatestReleaseInfo(func(url string) ([]byte, error) {
		if url != releaseURL {
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
		return []byte(`{"name":"Octopus dev-4a8f7da","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-07T01:11:40Z"}`), nil
	}, releaseURL)
	if err != nil {
		t.Fatalf("获取 GitHub Release 版本失败：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("GitHub Release 版本不符合预期：got %q, want %q", info.TagName, "dev-4a8f7da")
	}
	if info.PublishedAt != "2026-08-07T01:11:40Z" {
		t.Fatalf("应使用 updated_at 作为发布时间：got %q, want %q", info.PublishedAt, "2026-08-07T01:11:40Z")
	}
}

func TestGetLatestInfoSelectsSourceByVersionShape(t *testing.T) {
	const latestTagURL = "https://hub.test/tags/latest"
	const tagsURL = "https://hub.test/tags?page_size=100"
	const releaseURL = "https://api.test/releases/tags/octopus-binaries"

	// Docker 源较新、Release 源较旧：dev-* 当前版本应返回 Docker 源
	info, err := getLatestInfoFor("dev-4a8f7da", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-07T01:00:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}]}`), nil
		case releaseURL:
			return []byte(`{"name":"Octopus dev-2222222","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-06T00:00:00Z"}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("获取最新版本失败：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("dev-* 当前版本应使用 Docker 源：got %q, want %q", info.TagName, "dev-4a8f7da")
	}

	// Release 源较新、Docker 源较旧：dev-* 当前版本仍应返回 Docker 源（v* 发布不更新 Docker latest）
	info, err = getLatestInfoFor("dev-4a8f7da", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-01T00:00:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}]}`), nil
		case releaseURL:
			return []byte(`{"name":"Octopus v1.2.3","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-07T01:11:40Z"}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("获取最新版本失败：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("dev-* 当前版本不应被较新的 Release 源干扰：got %q, want %q", info.TagName, "dev-4a8f7da")
	}

	// v* 当前版本应使用 Release 源
	info, err = getLatestInfoFor("v1.2.0", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-07T01:00:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}]}`), nil
		case releaseURL:
			return []byte(`{"name":"Octopus v1.2.3","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-07T01:11:40Z"}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("获取最新版本失败：%v", err)
	}
	if info.TagName != "v1.2.3" {
		t.Fatalf("v* 当前版本应使用 Release 源：got %q, want %q", info.TagName, "v1.2.3")
	}

	// 未知形态（源码运行）取较新者
	info, err = getLatestInfoFor("dev", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-01T00:00:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}]}`), nil
		case releaseURL:
			return []byte(`{"name":"Octopus v1.2.3","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-07T01:11:40Z"}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("获取最新版本失败：%v", err)
	}
	if info.TagName != "v1.2.3" {
		t.Fatalf("未知形态应取较新源：got %q, want %q", info.TagName, "v1.2.3")
	}
}

func TestGetLatestInfoFallsBackWhenSourceFails(t *testing.T) {
	const latestTagURL = "https://hub.test/tags/latest"
	const tagsURL = "https://hub.test/tags?page_size=100"
	const releaseURL = "https://api.test/releases/tags/octopus-binaries"

	// GitHub 源失败时回退 Docker Hub
	info, err := getLatestInfoFor("v1.2.0", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return []byte(`{"name":"latest","last_updated":"2026-08-02T10:30:00Z","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}`), nil
		case tagsURL:
			return []byte(`{"results":[{"name":"sha-4a8f7da","images":[{"digest":"sha256:amd64"},{"digest":"sha256:arm64"}]}]}`), nil
		case releaseURL:
			return nil, fmt.Errorf("GitHub API 不可用")
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("GitHub 源失败时应回退 Docker Hub：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("回退版本不符合预期：got %q, want %q", info.TagName, "dev-4a8f7da")
	}

	// Docker Hub 源失败时回退 GitHub Releases
	info, err = getLatestInfoFor("dev-4a8f7da", func(url string) ([]byte, error) {
		switch url {
		case latestTagURL:
			return nil, fmt.Errorf("Docker Hub 不可用")
		case tagsURL:
			return nil, fmt.Errorf("Docker Hub 不可用")
		case releaseURL:
			return []byte(`{"name":"Octopus dev-4a8f7da","published_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-07T01:11:40Z"}`), nil
		default:
			return nil, fmt.Errorf("未预期的请求地址：%s", url)
		}
	}, latestTagURL, tagsURL, releaseURL)
	if err != nil {
		t.Fatalf("Docker Hub 源失败时应回退 GitHub Releases：%v", err)
	}
	if info.TagName != "dev-4a8f7da" {
		t.Fatalf("回退版本不符合预期：got %q, want %q", info.TagName, "dev-4a8f7da")
	}

	// 两源都失败时返回合并错误
	_, err = getLatestInfoFor("dev-4a8f7da", func(url string) ([]byte, error) {
		return nil, fmt.Errorf("网络不可用")
	}, latestTagURL, tagsURL, releaseURL)
	if err == nil {
		t.Fatal("两源都失败时应返回错误")
	}
}

func TestReleaseVersion(t *testing.T) {
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{name: "Octopus dev-4a8f7da", want: "dev-4a8f7da", ok: true},
		{name: "Octopus v1.2.3", want: "v1.2.3", ok: true},
		{name: "no-prefix", ok: false},
		{name: "Octopus ", ok: false},
	}
	for _, tt := range tests {
		got, ok := releaseVersion(tt.name)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("releaseVersion(%q) = (%q, %t), want (%q, %t)", tt.name, got, ok, tt.want, tt.ok)
		}
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
