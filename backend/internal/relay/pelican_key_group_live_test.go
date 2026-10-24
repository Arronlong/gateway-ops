package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 仅在显式指定渠道时运行；只读管理与元信息接口，不发起模型生成、不修改业务数据。
func TestPelicanKeyGroupLiveReadOnly(t *testing.T) {
	ids := os.Getenv("PELICAN_CHECK_CHANNEL_IDS")
	if ids == "" {
		t.Skip("set PELICAN_CHECK_CHANNEL_IDS for read-only upstream verification")
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open verification database")
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	cipher, err := crypto.NewCipher(os.Getenv("APP_SECRET"))
	if err != nil {
		t.Fatal(err)
	}
	repo := storage.NewRelayStations(db)
	service := NewService(repo, cipher, storage.NewChannels(db))
	if ids == "all" {
		var selected []uint
		db.Model(&storage.Channel{}).Order("id").Pluck("id", &selected)
		var values []string
		for _, id := range selected {
			values = append(values, strconv.Itoa(int(id)))
		}
		ids = strings.Join(values, ",")
	}
	for _, value := range strings.Split(ids, ",") {
		id, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal("invalid channel ID")
		}
		t.Run(fmt.Sprintf("channel-%d", id), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			var binding struct {
				Name           string
				Type           string
				SiteURL        string
				CredentialMode string
				StationID      uint
				AccountID      int64
				SessionID      uint
			}
			err := db.Raw(`SELECT channels.name,channels.type,channels.site_url,channels.credential_mode,links.relay_station_id AS station_id,links.relay_account_external_id AS account_id,CASE WHEN accounts.is_primary THEN accounts.channel_id ELSE accounts.id + ? END AS session_id FROM channels JOIN channel_accounts accounts ON accounts.channel_id=channels.id JOIN channel_relay_account_links links ON links.channel_account_id=accounts.id JOIN relay_pelican_results results ON results.relay_station_id=links.relay_station_id AND results.account_external_id=links.relay_account_external_id WHERE channels.id=? AND channels.deleted_at IS NULL AND accounts.deleted_at IS NULL AND results.finished_at >= ? ORDER BY results.finished_at DESC LIMIT 1`, storage.AdditionalAccountSessionOffset, id, pelicanLiveAuditAfter()).Scan(&binding).Error
			if err != nil || binding.StationID == 0 {
				t.Skip("no recent artwork for this channel")
			}
			session, _ := storage.NewAuthSessions(db).FindByChannel(binding.SessionID)
			t.Logf("渠道=%s 类型=%s 凭据模式=%s 已缓存会话=%v", binding.Name, binding.Type, binding.CredentialMode, session != nil)
			if session != nil {
				t.Logf("缓存会话 access_token=%v cookie=%v 已过期=%v", session.AccessTokenCipher != "", session.CookieCipher != "", session.ExpiresAt != nil && session.ExpiresAt.Before(time.Now()))
			}
			station, err := repo.FindByID(binding.StationID)
			if err != nil {
				t.Fatal("station unavailable")
			}
			key, err := cipher.Decrypt(station.APIKeyCipher)
			if err != nil {
				t.Fatal("cannot decrypt station credential")
			}
			var result storage.PelicanResult
			if db.Where("relay_station_id=? AND account_external_id=?", binding.StationID, binding.AccountID).Order("finished_at DESC").First(&result).Error != nil {
				t.Fatal("recent result unavailable")
			}
			target, err := service.preparePelican(ctx, *station, key, storage.PelicanConfig{AccountExternalID: binding.AccountID, Model: result.Model})
			if err != nil {
				t.Fatal("cannot prepare read-only target: " + redactTestText(err.Error(), key))
			}
			defer target.close()
			transport := target.client.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			target.client.Transport = pelicanKeyTraceTransport{base: transport, t: t}
			t.Logf("audit target station=%d account=%d artwork=%d", binding.StationID, binding.AccountID, result.ID)
			t.Logf("元信息查询地址=%s；只查询关联渠道，不调用生成接口", binding.SiteURL)
			info := service.pelicanKeyGroup(ctx, target, binding.StationID, binding.AccountID)
			t.Logf("查询分组=%q 已取得倍率=%v", info.Name, info.Multiplier != nil)
			if info.Name == "" || info.Multiplier == nil {
				t.Error("actual key group metadata unavailable")
			} else {
				t.Logf("只读验证倍率=%g", *info.Multiplier)
			}
		})
	}
}

type pelicanKeyTraceTransport struct {
	base http.RoundTripper
	t    *testing.T
}

func (p pelicanKeyTraceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	started := time.Now()
	response, err := p.base.RoundTrip(r)
	status := 0
	if response != nil {
		status = response.StatusCode
		if strings.Contains(r.URL.Path, "/keys") || strings.Contains(r.URL.Path, "/billing") {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			response.Body.Close()
			response.Body = io.NopCloser(bytes.NewReader(body))
			if readErr == nil {
				var raw map[string]json.RawMessage
				if json.Unmarshal(body, &raw) == nil {
					keys := make([]string, 0, len(raw))
					for key := range raw {
						keys = append(keys, key)
					}
					p.t.Logf("response fields=%v", keys)
					var data struct {
						Items []map[string]json.RawMessage `json:"items"`
					}
					if json.Unmarshal(raw["data"], &data) == nil && len(data.Items) > 0 {
						fields := make([]string, 0, len(data.Items[0]))
						for key := range data.Items[0] {
							fields = append(fields, key)
						}
						p.t.Logf("item fields=%v", fields)
					}
				}
			}
		}
	}
	p.t.Logf("元信息接口=%s 方法=%s HTTP=%d 耗时=%s 网络错误=%v", r.URL.Path, r.Method, status, time.Since(started).Round(time.Millisecond), err != nil)
	return response, err
}

func pelicanLiveAuditAfter() time.Time {
	if value, err := time.Parse(time.RFC3339, os.Getenv("PELICAN_CHECK_AFTER")); err == nil {
		return value
	}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Now().In(zone)
	return time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, zone)
}
