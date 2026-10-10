package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

var ledgerCategories = map[string]string{
	"user_revenue":      "income",
	"other_income":      "income",
	"account_purchase":  "expense",
	"upstream_recharge": "expense",
	"refund":            "expense",
	"purchase_refund":   "income",
	"operating_expense": "expense",
	"other_expense":     "expense",
}

func registerOperations(g *gin.RouterGroup, d *Deps) {
	gp := g.Group("/operations")
	gp.GET("/summary", func(c *gin.Context) { operationsSummary(c, d) })
	gp.GET("/ledger", func(c *gin.Context) { listOperationLedger(c, d) })
	gp.POST("/ledger", func(c *gin.Context) { createOperationLedger(c, d) })
	gp.PUT("/ledger/:id", func(c *gin.Context) { updateOperationLedger(c, d) })
	gp.DELETE("/ledger/:id", func(c *gin.Context) { deleteOperationLedger(c, d) })
	gp.GET("/local-accounts", func(c *gin.Context) { listLocalAccounts(c, d) })
	gp.POST("/local-accounts", func(c *gin.Context) { createLocalAccounts(c, d) })
	gp.PUT("/local-accounts/:id", func(c *gin.Context) { updateLocalAccount(c, d) })
	gp.DELETE("/local-accounts/:id", func(c *gin.Context) { deleteLocalAccount(c, d) })
	gp.POST("/local-accounts/auto-link", func(c *gin.Context) { autoLinkLocalAccounts(c, d) })
	gp.GET("/link-options", func(c *gin.Context) { operationLinkOptions(c, d) })
}

type operationSummaryView struct {
	Range     string                          `json:"range"`
	Ledger    storage.LedgerSummary           `json:"ledger"`
	Breakdown []storage.LedgerCategorySummary `json:"breakdown"`
	LocalPool storage.LocalAccountSummary     `json:"local_pool"`
	Cost      operationCostMetrics            `json:"cost"`
}

func operationsSummary(c *gin.Context, d *Deps) {
	since, rangeName := operationSince(c.DefaultQuery("range", "today"))
	ledger, breakdown, err := d.Operations.LedgerSummary(since)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	channels, err := d.Channels.List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	channelBalanceDecrease, err := d.Rates.ChannelConsumptionTotals(since)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	stationExpenses, err := d.Operations.StationExpenseTotals(since)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	stations, err := d.RelayStations.List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	costMetrics := operationCostMetrics{
		RelayCostComplete:     true,
		RelayCallCostComplete: true,
		RelayStations:         make([]operationRelayStationView, 0, len(stations)),
	}
	for _, channel := range channels {
		costMetrics.ChannelBalanceAmount += currentChannelBalance(channel)
		costMetrics.ChannelBalanceDecreaseAmount += channelBalanceDecrease[channel.ID]
	}
	successfulRelayCount := int64(0)
	for _, station := range stations {
		stationExpense := stationExpenses[station.ID]
		revenueUsage, revenueErr := d.Relay.UsageTotal(c.Request.Context(), station.ID, rangeName, since)
		if revenueErr != nil {
			if d.Log != nil {
				d.Log.Warn("operation relay revenue failed", "station_id", station.ID, "err", revenueErr)
			}
			costMetrics.RelayCostComplete = false
		} else {
			successfulRelayCount++
		}

		costUsage, costErr := d.Relay.UsageStats(c.Request.Context(), station.ID, rangeName, since)
		stationCost := relayCostSummaryFromUsage(costUsage)
		if revenueErr == nil {
			stationCost.UserCharge = revenueUsage.UserCharge
		}
		if costErr != nil {
			stationCost.Complete = false
			if stationCost.FailedAccounts == 0 {
				stationCost.FailedAccounts = 1
			}
			costMetrics.RelayCallCostComplete = false
		}
		stationComplete := revenueErr == nil && costErr == nil && stationCost.Complete
		if !stationCost.Complete {
			costMetrics.RelayCallCostComplete = false
		}
		stationActualCost := operationActualCost(stationExpense.AccountPurchase, stationExpense.UpstreamRecharge, 0)
		costMetrics.RelayRevenueAmount += stationCost.UserCharge
		costMetrics.RelayCallCostAmount += stationCost.Cost
		costMetrics.RelayFailedAccounts += stationCost.FailedAccounts
		costMetrics.RelayUnknownCostAccounts += stationCost.UnknownCostAccounts
		costMetrics.RelayStations = append(costMetrics.RelayStations, operationRelayStationView{
			ID: station.ID, Name: station.Name, UserChargeAmount: stationCost.UserCharge,
			CallCostAmount: stationCost.Cost, AccountPurchaseAmount: stationExpense.AccountPurchase,
			UpstreamRechargeAmount: stationExpense.UpstreamRecharge, ActualCostAmount: stationActualCost,
			Complete: stationComplete, FailedAccounts: stationCost.FailedAccounts,
			UnknownCostAccounts: stationCost.UnknownCostAccounts,
		})
	}
	ledger.RelayRevenueAmount = costMetrics.RelayRevenueAmount
	costMetrics.RelayAccountPurchaseAmount = ledger.AccountPurchaseAmount
	costMetrics.RelayUpstreamRechargeAmount = ledger.UpstreamRechargeAmount
	costMetrics.RelayActualCostAmount = operationActualCost(costMetrics.RelayAccountPurchaseAmount, costMetrics.RelayUpstreamRechargeAmount, costMetrics.ChannelBalanceAmount)
	costMetrics.ChannelRechargeAmount = ledger.UpstreamRechargeAmount
	costMetrics.RealtimeMultiplier = operationMultiplier(costMetrics.RelayRevenueAmount, costMetrics.RelayActualCostAmount)
	costMetrics.ChannelCostVariance = costMetrics.ChannelRechargeAmount - costMetrics.RelayCallCostAmount
	ledger.IncomeAmount += ledger.RelayRevenueAmount
	ledger.NetAmount += ledger.RelayRevenueAmount
	if ledger.RelayRevenueAmount > 0 {
		breakdown = append(breakdown, storage.LedgerCategorySummary{Direction: "income", Category: "relay_usage_revenue", Amount: ledger.RelayRevenueAmount, Count: successfulRelayCount})
	}
	_, pool, err := d.Operations.ListLocalAccounts(storage.LocalAccountFilter{})
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": operationSummaryView{
		Range: rangeName, Ledger: ledger, Breakdown: breakdown, LocalPool: pool, Cost: costMetrics,
	}})
}

func listOperationLedger(c *gin.Context, d *Deps) {
	since, _ := operationSince(c.DefaultQuery("range", "30d"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "300"))
	list, err := d.Operations.ListLedger(storage.LedgerFilter{
		Since: since, Direction: c.Query("direction"), Category: c.Query("category"), Query: c.Query("q"), Limit: limit,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

type ledgerInput struct {
	Direction      string  `json:"direction" binding:"required"`
	Source         string  `json:"source"`
	Category       string  `json:"category" binding:"required"`
	Amount         float64 `json:"amount" binding:"required"`
	Description    string  `json:"description" binding:"required"`
	ChannelID      *uint   `json:"channel_id"`
	RelayStationID *uint   `json:"relay_station_id"`
	OccurredAt     string  `json:"occurred_at" binding:"required"`
}

func createOperationLedger(c *gin.Context, d *Deps) {
	entry, err := bindLedgerInput(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Operations.CreateLedger(entry); err != nil {
		fail(c, operationErrorStatus(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entry})
}

func updateOperationLedger(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	entry, err := bindLedgerInput(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	entry.ID = id
	if err := d.Operations.UpdateLedger(entry); err != nil {
		fail(c, operationErrorStatus(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entry})
}

func deleteOperationLedger(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Operations.DeleteLedger(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		fail(c, status, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func bindLedgerInput(c *gin.Context) (*storage.OperationLedgerEntry, error) {
	var in ledgerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		return nil, err
	}
	direction := strings.ToLower(strings.TrimSpace(in.Direction))
	category := strings.ToLower(strings.TrimSpace(in.Category))
	source := strings.ToLower(strings.TrimSpace(in.Source))
	if source == "" {
		source = storage.LedgerSourceManual
	}
	if source != storage.LedgerSourceManual && source != storage.LedgerSourceHistoryImport {
		return nil, errors.New("账本来源无效")
	}
	expectedDirection, ok := ledgerCategories[category]
	if !ok {
		return nil, errors.New("账本类别无效")
	}
	if direction != expectedDirection {
		return nil, fmt.Errorf("%s 类别必须记为%s", category, map[string]string{"income": "收入", "expense": "支出"}[expectedDirection])
	}
	if in.Amount <= 0 {
		return nil, errors.New("金额必须大于 0")
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		return nil, errors.New("说明不能为空")
	}
	occurredAt, err := parseOperationTime(in.OccurredAt)
	if err != nil {
		return nil, errors.New("发生时间格式无效")
	}
	return &storage.OperationLedgerEntry{
		Direction: direction, Category: category, Amount: in.Amount, Currency: "CNY", Source: source,
		Description: description, ChannelID: in.ChannelID, RelayStationID: in.RelayStationID,
		OccurredAt: occurredAt,
	}, nil
}

type localAccountInput struct {
	Name                   string  `json:"name" binding:"required"`
	Identifier             string  `json:"identifier" binding:"required"`
	Platform               string  `json:"platform" binding:"required"`
	AccountType            string  `json:"account_type"`
	Status                 string  `json:"status"`
	PurchaseCost           float64 `json:"purchase_cost"`
	ExpectedQuota          float64 `json:"expected_quota"`
	PurchasedAt            string  `json:"purchased_at" binding:"required"`
	ExpiresAt              string  `json:"expires_at"`
	Notes                  string  `json:"notes"`
	RelayStationID         *uint   `json:"relay_station_id"`
	RelayAccountExternalID *int64  `json:"relay_account_external_id"`
}

type localAccountsInput struct {
	Accounts []localAccountInput `json:"accounts" binding:"required,min=1,max=500"`
}

func listLocalAccounts(c *gin.Context, d *Deps) {
	list, summary, err := d.Operations.ListLocalAccounts(storage.LocalAccountFilter{
		Status: c.Query("status"), Platform: c.Query("platform"), Query: c.Query("q"),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	platforms, err := d.Operations.ListLocalAccountPlatforms()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"accounts": list, "summary": summary, "platforms": platforms}})
}

func createLocalAccounts(c *gin.Context, d *Deps) {
	var in localAccountsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	accounts := make([]storage.LocalAccount, 0, len(in.Accounts))
	for _, raw := range in.Accounts {
		account, err := localAccountFromInput(raw)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		accounts = append(accounts, *account)
	}
	if err := d.Operations.CreateLocalAccounts(accounts); err != nil {
		fail(c, operationErrorStatus(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"created": len(accounts)}})
}

func updateLocalAccount(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	current, err := d.Operations.FindLocalAccount(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	var in localAccountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	updated, err := localAccountFromInput(in)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	updated.ID = id
	updated.CreatedAt = current.CreatedAt
	updated.LinkedAt = current.LinkedAt
	if err := d.Operations.UpdateLocalAccount(updated); err != nil {
		fail(c, operationErrorStatus(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func deleteLocalAccount(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Operations.DeleteLocalAccount(id); err != nil {
		fail(c, operationErrorStatus(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func operationErrorStatus(err error) int {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, storage.ErrInvalidOperationInput) {
		return http.StatusBadRequest
	}
	if errors.Is(err, storage.ErrOperationConflict) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func autoLinkLocalAccounts(c *gin.Context, d *Deps) {
	result, err := d.Operations.AutoLinkLocalAccounts()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

type operationLinkAccount struct {
	ExternalID  int64    `json:"external_id"`
	Name        string   `json:"name"`
	Platform    string   `json:"platform"`
	AccountType string   `json:"account_type"`
	Status      string   `json:"status"`
	Schedulable bool     `json:"schedulable"`
	Cost        *float64 `json:"cost,omitempty"`
}

type operationLinkStation struct {
	ID       uint                   `json:"id"`
	Name     string                 `json:"name"`
	Accounts []operationLinkAccount `json:"accounts"`
}

func operationLinkOptions(c *gin.Context, d *Deps) {
	stations, err := d.RelayStations.List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	result := make([]operationLinkStation, 0, len(stations))
	for _, station := range stations {
		accounts, accountErr := d.RelayStations.ListAccounts(station.ID)
		if accountErr != nil {
			fail(c, http.StatusInternalServerError, accountErr)
			return
		}
		view := operationLinkStation{ID: station.ID, Name: station.Name, Accounts: make([]operationLinkAccount, 0, len(accounts))}
		for _, account := range accounts {
			view.Accounts = append(view.Accounts, operationLinkAccount{
				ExternalID: account.ExternalID, Name: account.Name, Platform: account.Platform,
				AccountType: account.Type, Status: account.Status, Schedulable: account.Schedulable,
				Cost: account.RateMultiplier,
			})
		}
		result = append(result, view)
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func localAccountFromInput(in localAccountInput) (*storage.LocalAccount, error) {
	name := strings.TrimSpace(in.Name)
	identifier := strings.TrimSpace(in.Identifier)
	platform := strings.TrimSpace(in.Platform)
	if name == "" || identifier == "" || platform == "" {
		return nil, errors.New("名称、唯一标识和平台不能为空")
	}
	if in.PurchaseCost < 0 || in.ExpectedQuota < 0 {
		return nil, errors.New("采购成本和预期额度不能小于 0")
	}
	purchasedAt, err := parseOperationTime(in.PurchasedAt)
	if err != nil {
		return nil, errors.New("采购时间格式无效")
	}
	var expiresAt *time.Time
	if strings.TrimSpace(in.ExpiresAt) != "" {
		parsed, parseErr := parseOperationTime(in.ExpiresAt)
		if parseErr != nil {
			return nil, errors.New("到期时间格式无效")
		}
		expiresAt = &parsed
	}
	return &storage.LocalAccount{
		Name: name, Identifier: identifier, Platform: platform, AccountType: in.AccountType,
		Status: in.Status, PurchaseCost: in.PurchaseCost, ExpectedQuota: in.ExpectedQuota,
		PurchasedAt: purchasedAt, ExpiresAt: expiresAt, Notes: in.Notes,
		RelayStationID: in.RelayStationID, RelayAccountExternalID: in.RelayAccountExternalID,
	}, nil
}

func parseOperationTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("invalid time")
}

func operationSince(raw string) (time.Time, string) {
	if raw == "all" {
		return time.Time{}, "all"
	}
	return dashboardSince(raw)
}
