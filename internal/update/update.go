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
	devVersionPrefix      = "dev-"
	shaTagPrefix          = "sha-"
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

// GetLatestInfo 返回 Docker Hub latest 镜像对应的开发版本。
func GetLatestInfo() (*LatestInfo, error) {
	return getLatestInfo(doRequestWithFallback, dockerHubLatestTagURL, dockerHubTagsURL)
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
