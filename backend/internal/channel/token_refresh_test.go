package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/guliansheng/gateway-ops/internal/connector/sub2api"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRefreshChannelTokenPersistsPrimaryAndAdditionalCredentials(t *testing.T) {
	for _, additional := range []bool{false, true} {
		t.Run(fmt.Sprint(additional), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/api/v1/auth/refresh" || r.Header.Get("Authorization") != "" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["refresh_token"] != "refresh-old" {
					t.Error("wrong refresh token")
				}
				fmt.Fprint(w, `{"code":0,"data":{"access_token":"access-new","refresh_token":"refresh-new","expires_in":3600}}`)
			}))
			defer server.Close()
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
			if err != nil {
				t.Fatal(err)
			}
			cipher, _ := crypto.NewCipher("test-channel-refresh-secret")
			raw, _ := json.Marshal(Sub2APITokenCredential{AccessToken: "access-old", RefreshToken: "refresh-old", ExpiresAt: time.Now().Add(24 * time.Hour)})
			encrypted, _ := cipher.Encrypt(string(raw))
			current := &storage.Channel{ID: 7, Type: storage.ChannelTypeSub2API, SiteURL: server.URL, CredentialMode: storage.CredentialModeToken, PasswordCipher: encrypted}
			if additional {
				current.AuthSessionKey = storage.AdditionalAccountSessionKey(22)
				mock.ExpectQuery(`SELECT .*channel_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "channel_id", "password_cipher", "credential_mode"}).AddRow(22, 7, encrypted, "token"))
				mock.ExpectExec(`UPDATE "channel_accounts"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "channel_accounts"`).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectQuery(`SELECT .*channels`).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "site_url", "password_cipher", "credential_mode"}).AddRow(7, "sub2api", server.URL, encrypted, "token"))
				mock.ExpectExec(`UPDATE "channels"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .*channel_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(22))
				mock.ExpectExec(`UPDATE "channel_accounts"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				mock.ExpectExec(`UPDATE "channels"`).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			service := NewService(storage.NewChannels(db), nil, nil, nil, cipher)
			refreshed, err := service.RefreshChannelToken(context.Background(), current)
			if err != nil || !refreshed || calls != 1 {
				t.Fatalf("refreshed=%v calls=%d err=%v", refreshed, calls, err)
			}
			saved, _ := cipher.Decrypt(current.PasswordCipher)
			var credential Sub2APITokenCredential
			if json.Unmarshal([]byte(saved), &credential) != nil || credential.AccessToken != "access-new" || credential.RefreshToken != "refresh-new" {
				t.Fatal("rotated credentials not persisted")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshChannelTokenSkipsNewAPIAndPasswordMode(t *testing.T) {
	for _, c := range []storage.Channel{{Type: storage.ChannelTypeNewAPI, CredentialMode: storage.CredentialModeToken}, {Type: storage.ChannelTypeSub2API, CredentialMode: storage.CredentialModePassword}} {
		updated, err := (&Service{}).RefreshChannelToken(context.Background(), &c)
		if updated || err != nil {
			t.Fatalf("unsupported credential changed: %v", err)
		}
	}
}

func TestRefreshChannelTokenFailureDoesNotReplaceCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/refresh" {
			t.Error("unexpected request")
		}
		http.Error(w, `{"message":"invalid refresh token"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	cipher, _ := crypto.NewCipher("test-channel-refresh-secret")
	encrypted, _ := cipher.Encrypt(`{"access_token":"unchanged-access","refresh_token":"invalid-refresh"}`)
	current := &storage.Channel{ID: 7, Type: storage.ChannelTypeSub2API, SiteURL: server.URL, CredentialMode: storage.CredentialModeToken, PasswordCipher: encrypted, AuthSessionKey: storage.AdditionalAccountSessionKey(22)}
	mock.ExpectQuery(`SELECT .*channel_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "password_cipher", "credential_mode"}).AddRow(22, encrypted, "token"))
	mock.ExpectExec(`UPDATE "channel_accounts" SET "last_error"`).WillReturnResult(sqlmock.NewResult(0, 1))
	updated, err := NewService(storage.NewChannels(db), nil, nil, nil, cipher).RefreshChannelToken(context.Background(), current)
	if updated || err == nil || current.PasswordCipher != encrypted {
		t.Fatal("failed refresh must preserve credential")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
