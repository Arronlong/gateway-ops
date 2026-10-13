// Package version 提供构建信息和带缓存的 GitHub 稳定版检查。
package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// 正式构建由 Dockerfile 的 Go 链接参数注入；默认值与当前稳定版保持一致。
var (
	Version   = "1.0.1"
	Revision  = "unknown"
	BuildTime = "unknown"
)

const Repository = "Arronlong/gateway-ops"
const latestReleaseURL = "https://api.github.com/repos/" + Repository + "/releases/latest"

type Info struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Revision   string `json:"revision"`
	BuildTime  string `json:"build_time"`
	Repository string `json:"repository"`
	Channel    string `json:"channel"`
}

func Current() Info {
	channel := "stable"
	if strings.Contains(Version, "-dev") {
		channel = "development"
	} else if strings.Contains(Version, "-") {
		channel = "prerelease"
	}
	return Info{Name: "GatewayOps", Version: Version, Revision: Revision, BuildTime: BuildTime, Repository: Repository, Channel: channel}
}

var fullVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func normalize(value string) (string, error) {
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	if !fullVersion.MatchString(value) || !semver.IsValid("v"+value) {
		return "", fmt.Errorf("invalid semantic version %q", value)
	}
	return value, nil
}

// Compare 按语义化版本比较；提交元数据不影响排序。
func Compare(current, latest string) (int, error) {
	a, err := normalize(current)
	if err != nil {
		return 0, err
	}
	b, err := normalize(latest)
	if err != nil {
		return 0, err
	}
	return semver.Compare("v"+a, "v"+b), nil
}

type CheckResult struct {
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version,omitempty"`
	UpdateAvailable *bool      `json:"update_available"`
	Status          string     `json:"status"`
	ReleaseURL      string     `json:"release_url,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	CheckedAt       time.Time  `json:"checked_at"`
	Error           string     `json:"error,omitempty"`
}

type Checker struct {
	info      Info
	client    *http.Client
	endpoint  string
	mu        sync.Mutex
	cached    *CheckResult
	cachedErr error
	expires   time.Time
}

func NewChecker(info Info, client *http.Client, endpoint string) *Checker {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	if endpoint == "" {
		endpoint = latestReleaseURL
	}
	return &Checker{info: info, client: client, endpoint: endpoint}
}

func (c *Checker) Info() Info { return c.info }

// Check 合并并发查询，正常结果缓存 5 分钟，失败结果缓存 1 分钟。
func (c *Checker) Check(ctx context.Context) (CheckResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Now().Before(c.expires) {
		return *c.cached, c.cachedErr
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := c.fetch(ctx)
	if errors.Is(err, context.Canceled) {
		return result, err
	}
	ttl := 5 * time.Minute
	if err != nil {
		ttl = time.Minute
	}
	c.cached, c.cachedErr, c.expires = &result, err, time.Now().Add(ttl)
	return result, err
}

func (c *Checker) fetch(ctx context.Context) (result CheckResult, err error) {
	result = CheckResult{CurrentVersion: c.info.Version, Status: "check_failed", CheckedAt: time.Now().UTC()}
	defer func() {
		if err != nil {
			result.Error = err.Error()
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "GatewayOps/"+c.info.Version)
	res, err := c.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("GitHub release query failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		result.Status = "no_release"
		return result, nil
	}
	if res.StatusCode != http.StatusOK {
		return result, fmt.Errorf("GitHub release query returned HTTP %d", res.StatusCode)
	}
	var release struct {
		TagName     string     `json:"tag_name"`
		HTMLURL     string     `json:"html_url"`
		PublishedAt *time.Time `json:"published_at"`
		Draft       bool       `json:"draft"`
		Prerelease  bool       `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&release); err != nil {
		return result, fmt.Errorf("invalid GitHub release response: %w", err)
	}
	latest, err := normalize(release.TagName)
	if err != nil {
		return result, err
	}
	if release.Draft || release.Prerelease || semver.Prerelease("v"+latest) != "" {
		return result, errors.New("GitHub latest release is not a stable version")
	}
	order, err := Compare(c.info.Version, latest)
	if err != nil {
		return result, err
	}
	available := order < 0
	result.LatestVersion, result.UpdateAvailable, result.PublishedAt = latest, &available, release.PublishedAt
	result.ReleaseURL = "https://github.com/" + Repository + "/releases/tag/" + url.PathEscape(release.TagName)
	result.Status = "up_to_date"
	if order < 0 {
		result.Status = "update_available"
	} else if order > 0 {
		result.Status = "current_newer"
	}
	return result, nil
}
