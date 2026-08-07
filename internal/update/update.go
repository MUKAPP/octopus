package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const (
	dockerHubLatestTagURL = "https://hub.docker.com/v2/repositories/mukmiuikong/octopus/tags/latest"
	dockerHubTagsURL      = "https://hub.docker.com/v2/repositories/mukmiuikong/octopus/tags?page_size=100&ordering=last_updated"
	// githubReleasesURL 是固定槽位 Release，由 binaries 工作流在每次构建时更新，承载二进制版本检查。
	githubReleasesURL       = "https://api.github.com/repos/MUKAPP/octopus/releases/tags/octopus-binaries"
	githubReleaseNamePrefix = "Octopus "
	devVersionPrefix        = "dev-"
	shaTagPrefix            = "sha-"
)

type LatestInfo struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
}

type dockerHubImage struct {
	Digest string `json:"digest"`
}

type dockerHubTag struct {
	Name        string           `json:"name"`
	LastUpdated string           `json:"last_updated"`
	Images      []dockerHubImage `json:"images"`
}

type dockerHubTagsResponse struct {
	Results []dockerHubTag `json:"results"`
}

type githubRelease struct {
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	UpdatedAt   string `json:"updated_at"`
}

type requestFunc func(string) ([]byte, error)

// doRequestWithFallback 优先直连，失败后使用已配置的代理。
func doRequestWithFallback(url string) ([]byte, error) {
	data, err := doRequest(url, false)
	if err == nil {
		return data, nil
	}
	log.Warnf("direct request failed, trying with proxy: %v", err)
	return doRequest(url, true)
}

func doRequest(url string, useProxy bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hc, err := client.GetHTTPClientSystemProxy(useProxy)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求：%w", err)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("请求 %s 返回异常状态 %s：%s", url, resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// GetLatestInfo 返回与当前部署对应的最新版本：容器部署（dev-*）检查 Docker Hub
// latest 镜像，二进制部署（v* 发布版）检查 GitHub Releases 槽位；未知形态取较新者。
// 不能全局按时间取较新：两个源并非总是同步更新（v* 发布不更新 Docker latest）。
func GetLatestInfo(current string) (*LatestInfo, error) {
	return getLatestInfoFor(current, doRequestWithFallback, dockerHubLatestTagURL, dockerHubTagsURL, githubReleasesURL)
}

func getLatestInfoFor(current string, fetch requestFunc, latestTagURL, tagsURL, releaseURL string) (*LatestInfo, error) {
	dockerInfo, dockerErr := getLatestInfo(fetch, latestTagURL, tagsURL)
	releaseInfo, releaseErr := getLatestReleaseInfo(fetch, releaseURL)
	if dockerErr != nil && releaseErr != nil {
		return nil, fmt.Errorf("Docker Hub：%v；GitHub Releases：%v", dockerErr, releaseErr)
	}
	if dockerErr != nil {
		return releaseInfo, nil
	}
	if releaseErr != nil {
		return dockerInfo, nil
	}
	switch {
	case strings.HasPrefix(current, "v"):
		return releaseInfo, nil
	case strings.HasPrefix(current, devVersionPrefix):
		return dockerInfo, nil
	}
	if publishedAtLater(releaseInfo.PublishedAt, dockerInfo.PublishedAt) {
		return releaseInfo, nil
	}
	return dockerInfo, nil
}

// getLatestReleaseInfo 读取固定槽位 Release 的标题（形如 "Octopus dev-xxxxxxx"）得到版本。
// 槽位 Release 被反复复用覆盖，published_at 固定为首次发布时间，只有 updated_at 反映每次更新。
func getLatestReleaseInfo(fetch requestFunc, releaseURL string) (*LatestInfo, error) {
	body, err := fetch(releaseURL)
	if err != nil {
		return nil, fmt.Errorf("获取 GitHub Releases 信息：%w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Releases 信息：%w", err)
	}

	version, ok := releaseVersion(release.Name)
	if !ok {
		return nil, fmt.Errorf("GitHub Release 标题缺少版本标识：%q", release.Name)
	}
	return &LatestInfo{TagName: version, PublishedAt: release.UpdatedAt}, nil
}

func releaseVersion(name string) (string, bool) {
	if !strings.HasPrefix(name, githubReleaseNamePrefix) {
		return "", false
	}
	version := strings.TrimPrefix(name, githubReleaseNamePrefix)
	return version, version != ""
}

// publishedAtLater 比较两个 RFC3339 时间；解析失败时视为不晚于对方。
func publishedAtLater(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	if errA != nil || errB != nil {
		return false
	}
	return ta.After(tb)
}

func getLatestInfo(fetch requestFunc, latestTagURL, tagsURL string) (*LatestInfo, error) {
	latestBody, err := fetch(latestTagURL)
	if err != nil {
		return nil, fmt.Errorf("获取 Docker Hub latest 标签：%w", err)
	}

	var latestTag dockerHubTag
	if err := json.Unmarshal(latestBody, &latestTag); err != nil {
		return nil, fmt.Errorf("解析 Docker Hub latest 标签：%w", err)
	}
	if latestTag.Name != "latest" || len(latestTag.Images) == 0 {
		return nil, fmt.Errorf("Docker Hub latest 标签无效")
	}

	tagsBody, err := fetch(tagsURL)
	if err != nil {
		return nil, fmt.Errorf("获取 Docker Hub 镜像标签列表：%w", err)
	}

	var tags dockerHubTagsResponse
	if err := json.Unmarshal(tagsBody, &tags); err != nil {
		return nil, fmt.Errorf("解析 Docker Hub 镜像标签列表：%w", err)
	}
	for _, tag := range tags.Results {
		version, ok := imageVersion(tag.Name)
		if ok && sameImageDigests(tag.Images, latestTag.Images) {
			return &LatestInfo{TagName: version, PublishedAt: latestTag.LastUpdated}, nil
		}
	}

	return nil, fmt.Errorf("没有与 Docker Hub latest 镜像匹配的开发标签")
}

func imageVersion(tagName string) (string, bool) {
	if strings.HasPrefix(tagName, devVersionPrefix) && len(tagName) > len(devVersionPrefix) {
		return tagName, true
	}
	if strings.HasPrefix(tagName, shaTagPrefix) && len(tagName) > len(shaTagPrefix) {
		return devVersionPrefix + strings.TrimPrefix(tagName, shaTagPrefix), true
	}
	return "", false
}

func sameImageDigests(left, right []dockerHubImage) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}

	digests := make(map[string]struct{}, len(left))
	for _, image := range left {
		if image.Digest == "" {
			return false
		}
		digests[image.Digest] = struct{}{}
	}
	if len(digests) != len(left) {
		return false
	}
	for _, image := range right {
		if _, ok := digests[image.Digest]; !ok {
			return false
		}
	}
	return true
}
