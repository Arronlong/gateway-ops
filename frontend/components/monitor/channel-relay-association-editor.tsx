"use client"

import { useState } from "react"
import { Check, ChevronDown, Link2 } from "lucide-react"
import { toast } from "sonner"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "@/components/ui/command"
import { apiFetch } from "@/lib/api"
import { associationValue } from "@/lib/channel-relay-association"
import type { Channel, ChannelAccount, ChannelRelayAssociationSnapshot } from "@/lib/api-types"
import { cn } from "@/lib/utils"

interface ChannelRelayAssociationEditorProps {
  channel: Channel
  snapshot: ChannelRelayAssociationSnapshot | null
  disabled?: boolean
  onChanged: () => Promise<void>
}

function channelAccounts(channel: Channel): Pick<ChannelAccount, "id" | "username">[] {
  if (channel.accounts?.length > 0) return channel.accounts
  return [{
    id: 0,
    username: channel.username,
  }]
}

function accountName(username: string, index: number) {
  return username.trim() || `账号 ${index + 1}`
}

export function ChannelRelayAssociationEditor({ channel, snapshot, disabled = false, onChanged }: ChannelRelayAssociationEditorProps) {
  const [busy, setBusy] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState<string | null>(null)
  const accounts = channelAccounts(channel)
  const links = snapshot?.links ?? []
  const stations = snapshot?.stations ?? []

  async function update(accountID: number, stationID: number, value: string) {
    const key = `${accountID}:${stationID}`
    setBusy(key)
    try {
      const body = value === "auto"
        ? { mode: "auto", relay_account_external_id: null }
        : value === "manual:none"
          ? { mode: "manual", relay_account_external_id: null }
          : { mode: "manual", relay_account_external_id: Number(value.slice("manual:".length)) }
      await apiFetch(`/channels/${channel.id}/accounts/${accountID}/relay-links/${stationID}`, {
        method: "PUT",
        body: JSON.stringify(body),
      })
      await onChanged()
      toast.success("关联关系已更新")
    } catch (error) {
      toast.error((error as Error).message || "更新关联失败")
    } finally {
      setBusy(null)
    }
  }

  return (
    <section className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            <Link2 className="size-3.5 text-muted-foreground" />
            <Label className="text-sm font-semibold">关联中转站账号</Label>
          </div>
          <p className="mt-1 text-[11px] leading-5 text-muted-foreground">
            可按渠道账号分别关联中转站账号。列表筛选会按中转站聚合，不区分具体账号。
          </p>
        </div>
        <span className="shrink-0 rounded bg-muted px-2 py-1 text-[10px] text-muted-foreground">编辑后即时生效</span>
      </div>

      {stations.length === 0 ? (
        <div className="rounded-md border border-dashed border-border px-3 py-5 text-center text-xs text-muted-foreground">
          暂无可关联的中转站账号，请先在中转站管理中同步账号。
        </div>
      ) : (
        <div className="space-y-3">
          {accounts.map((account, accountIndex) => (
            <div key={account.id || accountIndex} className="space-y-2 rounded-md border border-border bg-background p-3">
              <div className="flex items-center justify-between gap-3">
                <p className="truncate text-xs font-medium">{accountName(account.username, accountIndex)}</p>
                <span className="shrink-0 text-[10px] text-muted-foreground">渠道账号</span>
              </div>
              <div className="space-y-2">
                {stations.map((station) => {
                  const link = links.find((item) => item.channel_account_id === account.id && item.relay_station_id === station.id)
                  const value = associationValue(link)
                  const key = `${account.id}:${station.id}`
                  const status = link?.linked
                    ? `${link.mode === "auto" ? "自动" : "手动"} · ${link.relay_account_name || `#${link.relay_account_external_id}`}`
                    : link?.mode === "manual" ? "手动 · 未关联" : "自动 · 未匹配"
                  const selectedRelayAccount = link?.linked
                    ? station.accounts.find((item) => item.external_id === link.relay_account_external_id)
                    : undefined
                  const selectedLabel = value === "auto"
                    ? "自动关联"
                    : value === "manual:none"
                      ? "未关联（手动）"
                      : selectedRelayAccount?.name || link?.relay_account_name || `账号 #${link?.relay_account_external_id ?? "-"}`
                  return (
                    <div key={station.id} className="grid items-start gap-2 rounded-md bg-muted/30 p-2 sm:grid-cols-[minmax(0,1fr)_12rem]">
                      <div className="min-w-0">
                        <p className="truncate text-xs font-medium" title={station.name}>{station.name}</p>
                        <p className={cn("mt-0.5 break-words text-[10px] leading-4", link?.linked ? "text-success" : "text-muted-foreground")} title={status}>{status}</p>
                      </div>
                      <Popover open={pickerOpen === key} onOpenChange={(open) => setPickerOpen(open ? key : null)}>
                        <PopoverTrigger asChild>
                          <Button
                            type="button"
                            variant="outline"
                            role="combobox"
                            aria-expanded={pickerOpen === key}
                            aria-label={`${station.name}中转账号关联`}
                            disabled={disabled || busy === key || account.id === 0}
                            className="h-9 min-w-0 w-full max-w-full justify-between overflow-hidden px-3 text-xs font-normal"
                            title={selectedLabel}
                          >
                            <span className="min-w-0 flex-1 truncate text-left">{selectedLabel}</span>
                            <ChevronDown className="size-4 shrink-0 opacity-50" />
                          </Button>
                        </PopoverTrigger>
                        <PopoverContent align="end" className="w-[min(32rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] p-0">
                          <Command>
                            <CommandInput placeholder="搜索账号名称、身份、地址或 ID" />
                            <CommandList className="max-h-72">
                              <CommandEmpty>没有匹配的中转站账号</CommandEmpty>
                              <CommandGroup heading="关联方式">
                                <CommandItem
                                  value="auto"
                                  keywords={["自动关联", "自动"]}
                                  onSelect={() => {
                                    setPickerOpen(null)
                                    if (value !== "auto") void update(account.id, station.id, "auto")
                                  }}
                                >
                                  <span className="min-w-0 flex-1">自动关联</span>
                                  <Check className={cn("size-4", value === "auto" ? "opacity-100" : "opacity-0")} />
                                </CommandItem>
                                <CommandItem
                                  value="manual:none"
                                  keywords={["未关联", "手动"]}
                                  onSelect={() => {
                                    setPickerOpen(null)
                                    if (value !== "manual:none") void update(account.id, station.id, "manual:none")
                                  }}
                                >
                                  <span className="min-w-0 flex-1">未关联（手动）</span>
                                  <Check className={cn("size-4", value === "manual:none" ? "opacity-100" : "opacity-0")} />
                                </CommandItem>
                              </CommandGroup>
                              <CommandSeparator />
                              <CommandGroup heading={`手动关联账号 · ${station.accounts.length}`}>
                                {station.accounts.map((relayAccount) => {
                                  const optionValue = `manual:${relayAccount.external_id}`
                                  const identity = relayAccount.identity?.trim() || "未提供身份"
                                  const address = relayAccount.base_url?.trim() || "未提供地址"
                                  return (
                                    <CommandItem
                                      key={relayAccount.external_id}
                                      value={optionValue}
                                      keywords={[relayAccount.name, relayAccount.identity ?? "", relayAccount.base_url ?? "", String(relayAccount.external_id)]}
                                      className="items-start py-2"
                                      onSelect={() => {
                                        setPickerOpen(null)
                                        if (value !== optionValue) void update(account.id, station.id, optionValue)
                                      }}
                                    >
                                      <div className="min-w-0 flex-1">
                                        <p className="truncate text-xs font-medium" title={relayAccount.name}>{relayAccount.name}</p>
                                        <p className="mt-0.5 truncate text-[10px] text-muted-foreground" title={`${identity} · ${address} · #${relayAccount.external_id}`}>
                                          {identity} · {address} · #{relayAccount.external_id}
                                        </p>
                                      </div>
                                      <Check className={cn("mt-0.5 size-4", value === optionValue ? "opacity-100" : "opacity-0")} />
                                    </CommandItem>
                                  )
                                })}
                              </CommandGroup>
                            </CommandList>
                          </Command>
                        </PopoverContent>
                      </Popover>
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      )}

      {accounts.some((account) => account.id === 0) ? (
        <p className="text-[11px] text-warning">该渠道账号尚未完成账号迁移，暂时不能设置关联；请先保存并重新打开编辑窗口。</p>
      ) : null}
    </section>
  )
}
