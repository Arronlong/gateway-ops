import { usageIntegrityComparableCount, usageIntegrityDifference, usageIntegrityLabel, usageIntegrityWindows, type UsageIntegrity } from "@/lib/usage-integrity"
import { cn } from "@/lib/utils"

export function UsageIntegrityReport({ result, loading }: { result: UsageIntegrity | null; loading: boolean }) {
  const windows = usageIntegrityWindows(result)
  const difference = result?.status === "mismatch"
  const interrupted = result?.phase === "interrupted" || result?.phase === "paused"
  const overall = difference ? "\u53d1\u73b0\u603b\u4f53\u5dee\u5f02" : usageIntegrityLabel(result, loading)
  const start = result?.since.startsWith("0001-") ? "\u5168\u90e8\u5386\u53f2" : result ? new Date(result.since).toLocaleString() : ""
  const end = result ? new Date(new Date(result.until).getTime()-1).toLocaleString() : ""
  const levelNames: Record<string,string> = {month:"\u6708",week:"\u5468",day:"\u5929",hour:"\u5c0f\u65f6"}
  const statusNames: Record<string,string> = {checking:"\u6838\u5bf9\u4e2d",repairing:"\u5904\u7406\u4e2d",repaired:"\u5df2\u4fee\u590d",verified:"\u5df2\u6838\u5bf9",interrupted:"\u672a\u5b8c\u6210",capped:"\u603b\u6570\u5f85\u786e\u8ba4"}
  return <div className="min-w-0 space-y-3 text-xs">
    <section className="min-w-0 rounded-md border border-border p-3" aria-label={"\u603b\u4f53\u6838\u5bf9"}>
      <h3 className="mb-2 font-medium">{"1. \u603b\u4f53\u6838\u5bf9"}</h3>
      <p className={cn("break-words font-medium", difference ? "text-warning" : interrupted ? "text-danger" : result?.status === "matched" ? "text-success" : "text-muted-foreground")}>{overall}</p>
      {result ? <>
        <div className="mt-2 grid grid-cols-3 gap-2 rounded-md bg-muted/40 p-2 tabular-nums">
          <div><p className="text-[11px] text-muted-foreground">{result.retained_count ? "\u672c\u5730\u53ef\u6838\u5bf9" : "\u672c\u5730\u6761\u6570"}</p><p className="mt-1 break-words font-medium">{usageIntegrityComparableCount(result)}</p></div>
          <div><p className="text-[11px] text-muted-foreground">{"\u4e0a\u6e38\u6761\u6570"}</p><p className="mt-1 break-words font-medium">{result.upstream_count ?? "\u672a\u53d6\u5f97"}</p></div>
          <div><p className="text-[11px] text-muted-foreground">{"\u672c\u5730\u5dee\u503c"}</p><p className={cn("mt-1 break-words font-medium",difference ? "text-warning" : "")}>{usageIntegrityDifference(usageIntegrityComparableCount(result),result.upstream_count)}</p></div>
        </div>
        {result.accounts.map(account=><p key={account.account_id} className="mt-2 break-words text-[11px] text-muted-foreground"><span className="font-medium text-foreground">{account.account_name}</span>{`\uff1a\u672c\u5730 ${usageIntegrityComparableCount(account)} / \u4e0a\u6e38 ${account.upstream_count ?? "\u672a\u53d6\u5f97"} \u00b7 ${usageIntegrityDifference(usageIntegrityComparableCount(account),account.upstream_count)}`}{account.retained_count ? `\uff1b\u53e6\u4fdd\u7559 ${account.retained_count} \u6761\u66f4\u65e9\u5386\u53f2\uff0c\u4e0a\u6e38\u5f53\u524d\u5df2\u4e0d\u53ef\u6838\u5bf9` : ""}</p>)}
        <p className="mt-2 break-words text-[11px] text-muted-foreground">{start}{" - "}{end}</p>
        <p className="break-words text-[11px] text-muted-foreground">{"\u6838\u5bf9\u65f6\u95f4\uff1a"}{new Date(result.checked_at).toLocaleString()}</p>
        {result.accounts.map(account=><p key={`checkpoint-${account.account_id}`} className="mt-1 break-words text-[11px] text-muted-foreground">{account.account_name}{"\uff1a\u5386\u53f2\u8fde\u7eed\u6838\u5bf9\u5b8c\u6210\u81f3 "}{account.history_verified_until ? new Date(account.history_verified_until).toLocaleString() : "\u5c1a\u672a\u786e\u8ba4"}</p>)}
      </> : null}
    </section>
    <section className="min-w-0 rounded-md border border-border p-3" aria-label={"\u5f02\u5e38\u533a\u95f4"}>
      <h3 className="mb-2 font-medium">{"2. \u5f02\u5e38\u533a\u95f4"}</h3>
      {windows.length ? <div className="max-h-64 space-y-2 overflow-y-auto">
        {windows.map((window,index)=><div key={`${window.account_id}-${window.since}-${window.level}-${index}`} className="rounded-md bg-muted/40 p-2 text-[11px]">
          <div className="flex flex-wrap items-center justify-between gap-1"><span className="break-words font-medium">{window.account_name}{window.level==="history" ? " \u00b7 \u5386\u53f2\u8303\u56f4\u6838\u5bf9" : ` \u00b7 \u6309${levelNames[window.level] ?? window.level}\u5b9a\u4f4d`}</span><span className={window.status==="repaired"||window.status==="verified"?"text-success":"text-warning"}>{statusNames[window.status] ?? (window.level==="day"||window.level==="hour"?"\u5f85\u4fee\u590d":"\u5f85\u7ec6\u5206")}</span></div>
          <p className="mt-1 break-words text-muted-foreground">{window.since.startsWith("0001-") ? "\u5168\u90e8\u5386\u53f2" : new Date(window.since).toLocaleString()}{" - "}{new Date(new Date(window.until).getTime()-1).toLocaleString()}</p>
          <p className="mt-1 break-words tabular-nums">{window.counts_pending || window.status==="checking" ? "\u6b63\u5728\u6838\u5bf9\u533a\u95f4\u6761\u6570\u2026" : window.total_capped ? `\u4e0a\u6e38\u8fd4\u56de 2000 \u6761\uff0c\u603b\u6570\u53ef\u80fd\u622a\u65ad\uff0c\u5c1a\u4e0d\u80fd\u5224\u5b9a\u5dee\u503c` : <>{window.status==="repaired" && window.level!=="history" ? "\u4fee\u590d\u524d\uff1a" : ""}{`\u672c\u5730 ${window.local_count} / \u4e0a\u6e38 ${window.upstream_count} \u00b7 ${usageIntegrityDifference(window.local_count,window.upstream_count)}`}</>}</p>
          {window.saved_pages ? <p className="mt-1 break-words text-muted-foreground">{`\u7d2f\u8ba1\u4fdd\u5b58 ${window.saved_pages} \u9875\u3001${window.saved_rows ?? 0} \u6761\u8c03\u7528`}{window.page ? ` \u00b7 \u6700\u8fd1\u5904\u7406\u7b2c ${window.page} \u9875` : ""}</p> : null}
        </div>)}
      </div> : <p className="break-words text-[11px] text-muted-foreground">{interrupted?"\u5b9a\u4f4d\u672a\u5b8c\u6210\uff0c\u5c1a\u672a\u786e\u8ba4\u5177\u4f53\u5f02\u5e38\u65e5\u671f\u3002":difference?(result?.phase==="complete" ? "\u603b\u4f53\u5df2\u786e\u8ba4\u5dee\u5f02\uff0c\u70b9\u51fb\u91cd\u65b0\u62c9\u53d6\u540e\u9010\u6bb5\u6838\u5bf9\u5e76\u4fee\u590d\u3002" : "\u603b\u4f53\u5df2\u786e\u8ba4\u5dee\u5f02\uff0c\u6b63\u5728\u9010\u6bb5\u6838\u5bf9\u5e76\u4fee\u590d\u3002"):result?.status==="matched"?"\u603b\u6570\u4e00\u81f4\uff0c\u65e0\u9700\u5206\u6bb5\u6838\u5bf9\u6216\u91cd\u65b0\u62c9\u53d6\u3002":"\u53d6\u5f97\u603b\u6570\u540e\uff0c\u4ec5\u5bf9\u5b58\u5728\u5dee\u5f02\u7684\u533a\u95f4\u7ee7\u7eed\u5b9a\u4f4d\u3002"}</p>}
      {interrupted && windows.length ? <p className="mt-2 break-words text-[11px] text-warning">{"\u4ee5\u4e0a\u4e3a\u5df2\u53d6\u5f97\u7684\u533a\u95f4\u6838\u5bf9\u660e\u7ec6\uff0c\u5176\u4f59\u533a\u95f4\u5c1a\u672a\u5b8c\u6210\u6838\u5bf9\u3002"}</p> : null}
    </section>
  </div>
}
