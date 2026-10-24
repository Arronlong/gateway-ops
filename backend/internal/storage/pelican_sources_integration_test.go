package storage

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPelicanSourcesPostgresIsolationAndRetention(t *testing.T) {
	dsn := os.Getenv("GATEWAYOPS_TEST_DSN")
	if dsn == "" {
		t.Skip("GATEWAYOPS_TEST_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.DB()
	defer connection.Close()
	rollback := errors.New("rollback isolated pelican test schema")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA pelican_sources_integration_test").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL search_path TO pelican_sources_integration_test").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&RelayStation{}, &RelayGroup{}, &RelayAccount{}, &Channel{}, &ChannelAccount{}, &ChannelRelayAccountLink{}, &PelicanConfig{}, &PelicanResult{}, &GroupPelicanConfig{}, &GroupPelicanResult{}, &AppSetting{}); err != nil {
			return err
		}
		repo := NewRelayStations(tx)
		settings := NewSettings(tx)
		if err := settings.Set(SettingPelicanHistoryCount, "2"); err != nil {
			return err
		}
		station := RelayStation{ID: 7, Name: "测试中转站", BaseURL: "https://relay.example", Type: "sub2api"}
		if err := tx.Create(&station).Error; err != nil {
			return err
		}
		for _, group := range []RelayGroup{{RelayStationID: 7, ExternalID: 20, Name: "公开销售组", MonitorEnabled: true}, {RelayStationID: 7, ExternalID: 21, Name: "监控中的专属组", IsExclusive: true, MonitorEnabled: true}, {RelayStationID: 7, ExternalID: 22, Name: "未监控专属组", IsExclusive: true, MonitorEnabled: false}} {
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		for _, account := range []RelayAccount{{RelayStationID: 7, ExternalID: 42, Name: "已开启号池", Type: "oauth", Platform: "openai"}, {RelayStationID: 7, ExternalID: 43, Name: "未开启号池", Type: "oauth", Platform: "openai"}, {RelayStationID: 7, ExternalID: 44, Name: "渠道 Key", Type: "apikey", Platform: "openai"}} {
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			config := PelicanConfig{RelayStationID: 7, AccountExternalID: account.ExternalID, Enabled: account.ExternalID != 43, Model: "test-model", Prompt: "draw", ReasoningEffort: "medium", ParallelCount: 1, CronExpression: "0 * * * *", NextRunAt: now}
			if err := repo.SavePelicanConfig(&config); err != nil {
				return err
			}
			if err := repo.SavePelicanResult(&PelicanResult{RelayStationID: 7, AccountExternalID: account.ExternalID, AccountName: account.Name, Model: "test-model", Status: "success", HTML: "<svg/>", StartedAt: now, FinishedAt: now}); err != nil {
				return err
			}
		}
		for _, id := range []int64{20, 21} {
			config := GroupPelicanConfig{RelayStationID: 7, GroupExternalID: id, Enabled: id == 20, CronExpression: "0 * * * *", NextRunAt: now}
			if err := repo.SaveGroupPelicanConfig(&config); err != nil {
				return err
			}
		}
		for index := 1; index <= 4; index++ {
			if err := repo.SaveGroupPelicanResult(&GroupPelicanResult{RelayStationID: 7, GroupExternalID: 20, GroupName: "公开销售组", Model: "model-a", ReasoningEffort: "high", Status: "success", HTML: "<svg/>", StartedAt: now, FinishedAt: now.Add(time.Duration(index) * time.Second)}); err != nil {
				return err
			}
		}
		if err := repo.SaveGroupPelicanResult(&GroupPelicanResult{RelayStationID: 7, GroupExternalID: 21, GroupName: "专属销售组", Status: "success", HTML: "private", StartedAt: now, FinishedAt: now}); err != nil {
			return err
		}
		history, err := repo.GroupPelicanGallery(7, 2)
		if err != nil {
			return err
		}
		if len(history) != 3 || history[0].HTML != "" {
			t.Fatalf("group gallery or retention: %#v", history)
		}
		public, err := repo.GroupPelicanGallery(7, 2, true)
		if err != nil {
			return err
		}
		if len(public) != 3 || public[0].GroupExternalID != 20 || public[2].GroupExternalID != 21 {
			t.Fatalf("monitored group artwork missing or unmonitored group leaked: %#v", public)
		}
		if repo.PublicPelicanGroup(7, 21) != nil {
			t.Fatal("a monitored group must share the public monitor visibility")
		}
		if repo.PublicPelicanGroup(7, 22) == nil {
			t.Fatal("unmonitored group must not be public")
		}
		if _, err := repo.GroupPelicanResult(7, 21, public[0].ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("cross-group result access allowed")
		}
		sources, err := repo.PelicanSourceGallery(2)
		if err != nil {
			return err
		}
		if len(sources) != 4 || sources[0].Kind != "pool_account" || sources[0].ExternalID != 42 || sources[1].Kind != "pool_account" || sources[1].ExternalID != 43 || sources[2].Kind != "relay_group" || sources[2].ExternalID != 20 || sources[3].Kind != "relay_group" || sources[3].ExternalID != 21 {
			t.Fatalf("enabled source selection: %#v", sources)
		}
		if sources[1].Config.(PelicanConfig).Enabled || sources[3].Config.(GroupPelicanConfig).Enabled {
			t.Fatal("disabled sources must remain visible so their automatic-test switches can be enabled")
		}
		encoded, _ := json.Marshal(sources)
		disabled, err := repo.PelicanSourceGallery(2, "group:7:21")
		if err != nil {
			return err
		}
		if len(disabled) != 1 || disabled[0].ExternalID != 21 {
			t.Fatalf("explicit history link should remain readable without enabling: %#v", disabled)
		}
		for _, field := range []string{`"html"`, `"prompt"`, `"api_key"`} {
			if strings.Contains(string(encoded), field) {
				t.Fatalf("sensitive or heavy gallery field: %s", field)
			}
		}
		ui := defaultUISettings()
		ui.PelicanHistoryCount = 1
		if err := settings.SaveUISettings(ui); err != nil {
			return err
		}
		history, err = repo.GroupPelicanGallery(7, 1, true)
		if err != nil {
			return err
		}
		if len(history) != 2 {
			t.Fatalf("settings did not physically prune group history: %#v", history)
		}
		var count int64
		if err := tx.Model(&GroupPelicanResult{}).Where("group_external_id = ?", 20).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("physical result count=%d", count)
		}
		config, err := repo.GroupPelicanConfig(7, 21)
		if err != nil {
			return err
		}
		if config.Enabled {
			t.Fatal("settings unexpectedly enabled group")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
