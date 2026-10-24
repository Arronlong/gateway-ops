package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPelicanKeySessionUsesLinkedAccountSessionAndExpiry(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(`(?s)SELECT channels.site_url, sessions.access_token_cipher.*accounts.deleted_at IS NULL.*channels.deleted_at IS NULL.*LEFT JOIN auth_sessions.*CASE WHEN accounts.is_primary THEN accounts.channel_id ELSE accounts.id \+.*sessions.expires_at > CURRENT_TIMESTAMP`).WithArgs(AdditionalAccountSessionOffset, uint(7), int64(20), "sub2api", "newapi").WillReturnRows(sqlmock.NewRows([]string{"site_url", "access_token_cipher"}).AddRow("https://upstream.example", "encrypted-session"))
	sessions, err := repo.PelicanKeySessions(context.Background(), 7, 20)
	if err != nil || len(sessions) != 1 || sessions[0].AccessTokenCipher != "encrypted-session" {
		t.Fatalf("sessions=%#v err=%v", sessions, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanGalleryReturnsStoredKeyGroupWithoutArtwork(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(`SELECT id, .*upstream_group_name, upstream_group_multiplier FROM`).WithArgs(uint(7), 30).WillReturnRows(sqlmock.NewRows([]string{"id", "upstream_group_name", "upstream_group_multiplier"}).AddRow(31, "历史分组", 0))
	results, err := repo.PelicanGallery(7)
	if err != nil || len(results) != 1 || results[0].UpstreamGroupName != "历史分组" || results[0].UpstreamGroupMultiplier == nil || *results[0].UpstreamGroupMultiplier != 0 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	encoded, _ := json.Marshal(results)
	if strings.Contains(string(encoded), `"html"`) || strings.Contains(string(encoded), `"api_key"`) {
		t.Fatal("gallery must not include artwork or keys")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
