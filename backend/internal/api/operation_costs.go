package api

import (
	"math"

	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

// operationMultiplier 返回实际综合成本相对于中转站用户消耗的倍率，汇率按 1:1 计算。
// 没有正的中转站用户消耗时不返回倍率，避免除零或误报有效结果。
func operationMultiplier(relayConsumption, actualCost float64) *float64 {
	if relayConsumption <= 0 || math.IsNaN(relayConsumption) || math.IsInf(relayConsumption, 0) {
		return nil
	}
	if actualCost < 0 || math.IsNaN(actualCost) || math.IsInf(actualCost, 0) {
		return nil
	}
	value := actualCost / relayConsumption
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

// operationActualCost returns cash expenses consumed after subtracting the current channel balance.
func operationActualCost(accountPurchase, upstreamRecharge, channelBalance float64) float64 {
	actualCost := 0.0
	if accountPurchase >= 0 && !math.IsNaN(accountPurchase) && !math.IsInf(accountPurchase, 0) {
		actualCost += accountPurchase
	}
	if upstreamRecharge >= 0 && !math.IsNaN(upstreamRecharge) && !math.IsInf(upstreamRecharge, 0) {
		actualCost += upstreamRecharge
	}
	if channelBalance >= 0 && !math.IsNaN(channelBalance) && !math.IsInf(channelBalance, 0) {
		actualCost -= channelBalance
	}
	if actualCost < 0 {
		return 0
	}
	return actualCost
}

// relayCostSummary 汇总一个中转站的用户收入和已知账号调用成本。
type relayCostSummary struct {
	UserCharge          float64
	Cost                float64
	FailedAccounts      int
	UnknownCostAccounts int
	Complete            bool
}

func relayCostSummaryFromUsage(usage relay.UsageSummary) relayCostSummary {
	summary := relayCostSummary{
		UserCharge:     usage.UserCharge,
		FailedAccounts: usage.FailedAccounts,
		Complete:       usage.FailedAccounts == 0,
	}
	for _, account := range usage.Accounts {
		if !account.AccountCostKnown {
			summary.UnknownCostAccounts++
			if account.AccountCost > 0 && !math.IsNaN(account.AccountCost) && !math.IsInf(account.AccountCost, 0) {
				summary.Cost += account.AccountCost
			}
			continue
		}
		summary.Cost += account.AccountCost
	}
	if summary.UnknownCostAccounts > 0 {
		summary.Complete = false
	}
	return summary
}

// operationRelayStationView 是成本管理中的中转站维度实时成本明细。
type operationRelayStationView struct {
	ID                     uint     `json:"id"`
	Name                   string   `json:"name"`
	UserChargeAmount       float64  `json:"user_charge_amount"`
	CallCostAmount         float64  `json:"call_cost_amount"`
	AccountPurchaseAmount  float64  `json:"account_purchase_amount"`
	UpstreamRechargeAmount float64  `json:"upstream_recharge_amount"`
	ActualCostAmount       float64  `json:"actual_cost_amount"`
	RealtimeMultiplier     *float64 `json:"realtime_multiplier,omitempty"`
	Complete               bool     `json:"complete"`
	FailedAccounts         int      `json:"failed_accounts"`
	UnknownCostAccounts    int      `json:"unknown_cost_accounts"`
}

// operationCostMetrics 汇总成本管理的多套成本口径。渠道支出取上游充值台账，
// 余额下降采样单独保留用于诊断，不与中转站调用成本相加。
type operationCostMetrics struct {
	RelayRevenueAmount           float64                     `json:"relay_revenue_amount"`
	RelayCallCostAmount          float64                     `json:"relay_call_cost_amount"`
	RelayAccountPurchaseAmount   float64                     `json:"relay_account_purchase_amount"`
	RelayUpstreamRechargeAmount  float64                     `json:"relay_upstream_recharge_amount"`
	RelayActualCostAmount        float64                     `json:"relay_actual_cost_amount"`
	RealtimeMultiplier           *float64                    `json:"realtime_multiplier,omitempty"`
	RelayCostComplete            bool                        `json:"relay_cost_complete"`
	RelayCallCostComplete        bool                        `json:"relay_call_cost_complete"`
	RelayFailedAccounts          int                         `json:"relay_failed_accounts"`
	RelayUnknownCostAccounts     int                         `json:"relay_unknown_cost_accounts"`
	ChannelBalanceAmount         float64                     `json:"channel_balance_amount"`
	ChannelRechargeAmount        float64                     `json:"channel_recharge_amount"`
	ChannelBalanceDecreaseAmount float64                     `json:"channel_balance_decrease_amount"`
	ChannelCostVariance          float64                     `json:"channel_cost_variance"`
	RelayStations                []operationRelayStationView `json:"relay_stations"`
}

func currentChannelBalance(channel storage.Channel) float64 {
	if channel.LastBalance != nil {
		return *channel.LastBalance
	}
	if channel.BalanceMode == storage.BalanceModeManual {
		return channel.ManualBalance
	}
	return 0
}
