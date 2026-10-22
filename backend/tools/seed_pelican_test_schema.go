package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/guliansheng/gateway-ops/internal/config"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

var schemaPattern = regexp.MustCompile(`^gatewayops_pelican_test_[a-f0-9]{12}$`)

func cloneRows[T any](tx *gorm.DB, now time.Time, recentColumn string, accountColumn string, accountID uint, sourceRows *gorm.DB, destination *gorm.DB) error {
	var rows []T
	query := sourceRows
	if recentColumn != "" {
		query = query.Where(recentColumn+" >= ?", now.Add(-time.Hour)).Order("id DESC").Limit(60)
	}
	if accountColumn != "" {
		query = query.Where(accountColumn+" = ?", accountID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Session(&gorm.Session{NewDB: true}).CreateInBatches(&rows, 500).Error
}

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatal(err)
	}
	schema := os.Getenv("GATEWAYOPS_DATABASE_SCHEMA")
	if !schemaPattern.MatchString(schema) {
		log.Fatal("refusing an invalid isolated test schema name")
	}
	sourceConfig := cfg.Database.ToStorageConfig()
	sourceConfig.SearchPath = "public"
	source, err := storage.Open(sourceConfig)
	if err != nil {
		log.Fatal(err)
	}
	sourceSQL, err := source.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sourceSQL.Close()
	quoted := "\"" + schema + "\""
	if err := source.Exec("CREATE SCHEMA IF NOT EXISTS " + quoted).Error; err != nil {
		log.Fatal(err)
	}
	testConfig := cfg.Database.ToStorageConfig()
	testConfig.SearchPath = schema
	destination, err := storage.Open(testConfig)
	if err != nil {
		log.Fatal(err)
	}
	destinationSQL, err := destination.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer destinationSQL.Close()
	if err := destination.Exec(`CREATE TABLE IF NOT EXISTS pelican_test_seed_state (id smallint PRIMARY KEY, seeded_at timestamptz NOT NULL)`).Error; err != nil {
		log.Fatal(err)
	}
	var seeded bool
	if err := destination.Raw(`SELECT EXISTS(SELECT 1 FROM pelican_test_seed_state WHERE id=1)`).Scan(&seeded).Error; err != nil {
		log.Fatal(err)
	}
	if seeded {
		fmt.Println("Isolated test schema is already initialized; preserving its data.")
		return
	}
	if err := storage.AutoMigrate(destination); err != nil {
		log.Fatal(err)
	}
	var channels []storage.Channel
	if err := source.Find(&channels).Error; err != nil {
		log.Fatal(err)
	}
	var channelAccounts []storage.ChannelAccount
	if err := source.Find(&channelAccounts).Error; err != nil {
		log.Fatal(err)
	}
	var stations []storage.RelayStation
	if err := source.Find(&stations).Error; err != nil {
		log.Fatal(err)
	}
	var groups []storage.RelayGroup
	if err := source.Find(&groups).Error; err != nil {
		log.Fatal(err)
	}
	var relayChannels []storage.RelayChannel
	if err := source.Find(&relayChannels).Error; err != nil {
		log.Fatal(err)
	}
	var relayAccounts []storage.RelayAccount
	if err := source.Find(&relayAccounts).Error; err != nil {
		log.Fatal(err)
	}
	now := time.Now().UTC()
	err = destination.Transaction(func(tx *gorm.DB) error {
		copies := []any{&channels, &channelAccounts, &stations, &groups, &relayChannels, &relayAccounts}
		for _, rows := range copies {
			if err := tx.Session(&gorm.Session{NewDB: true}).CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}
		if err := cloneRows[storage.RateSnapshot](tx, now, "", "", 0, source.Model(&storage.RateSnapshot{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.ChannelRelayAccountLink](tx, now, "", "", 0, source.Model(&storage.ChannelRelayAccountLink{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayChannelGroup](tx, now, "", "", 0, source.Model(&storage.RelayChannelGroup{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayChannelMapping](tx, now, "", "", 0, source.Model(&storage.RelayChannelMapping{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayAccountGroup](tx, now, "", "", 0, source.Model(&storage.RelayAccountGroup{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayAccountCostOverride](tx, now, "", "", 0, source.Model(&storage.RelayAccountCostOverride{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayAccountAdjustmentLog](tx, now, "", "", 0, source.Model(&storage.RelayAccountAdjustmentLog{}), destination); err != nil {
			return err
		}
		if err := cloneRows[storage.RelayChannelPricingChange](tx, now, "", "", 0, source.Model(&storage.RelayChannelPricingChange{}), destination); err != nil {
			return err
		}
		for _, account := range channelAccounts {
			if err := cloneRows[storage.ChannelUsageEvent](tx, now, "occurred_at", "channel_account_id", account.ID, source.Model(&storage.ChannelUsageEvent{}).Where("channel_id = ?", account.ChannelID), destination); err != nil {
				return err
			}
		}
		for _, account := range relayAccounts {
			if err := cloneRows[storage.RelayUsageEvent](tx, now, "occurred_at", "account_external_id", uint(account.ExternalID), source.Model(&storage.RelayUsageEvent{}).Where("relay_station_id = ?", account.RelayStationID), destination); err != nil {
				return err
			}
		}
		for _, table := range []string{"channels", "channel_accounts", "rate_snapshots", "channel_relay_account_links", "relay_stations", "relay_groups", "relay_channels", "relay_channel_mappings", "relay_accounts", "relay_usage_events", "relay_account_cost_overrides", "relay_account_adjustment_logs", "relay_channel_pricing_changes", "channel_usage_events"} {
			relation := quoted + `."` + table + `"`
			if err := tx.Exec(`SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM `+relation+`),1), EXISTS(SELECT 1 FROM `+relation+`))`, schema+"."+table).Error; err != nil {
				return err
			}
		}
		settings := storage.NewSettings(tx)
		for key, value := range map[string]string{
			storage.SettingChannelSyncConfigured: "true", storage.SettingChannelSyncEnabled: "false",
			storage.SettingRelayRateConfigured: "true", storage.SettingRelayRateEnabled: "false",
			storage.SettingRelaySnapshotConfigured: "true", storage.SettingRelaySnapshotEnabled: "false",
		} {
			if err := settings.Set(key, value); err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO pelican_test_seed_state(id,seeded_at) VALUES(1,?)`, now).Error
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Isolated schema %s initialized with %d channels, %d relay stations and %d accounts; recent usage samples copied for cache metrics.\n", schema, len(channels), len(stations), len(relayAccounts))
}
