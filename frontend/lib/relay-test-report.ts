import type { RelayTestRun } from "./api-types"
const escapeXML = (value: string) => value.replace(/[&<>"']/g, (char) => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&apos;"})[char]!)
export function relayTestReportSVG(run: RelayTestRun): string {
  const height = 210 + Math.max(run.results.length, 1) * 64
  const text = (x: number, y: number, value: string, size = 14, color = "#334155") => `<text x="${x}" y="${y}" fill="${color}" font-size="${size}">${escapeXML(value)}</text>`
  const rows = run.results.map((call, index) => {
    const y = 205 + index * 64, color = call.success ? "#059669" : "#dc2626"
    return `<path d="M 180 ${y} H 650" fill="none" stroke="${color}" stroke-width="2" class="flow"/>
    <circle cx="180" cy="${y}" r="5" fill="${color}"/><circle cx="650" cy="${y}" r="5" fill="${color}"/>
    ${text(40,y+5,`#${call.index}`)}${text(210,y-10,call.success ? "调用成功" : "调用失败",14,color)}
    ${text(680,y+5,`${call.duration_ms} ms`,14,color)}${text(805,y+5,call.status_code ? `HTTP ${call.status_code}` : "无 HTTP 响应",12,color)}`
  }).join("")
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 ${height}" role="img" aria-label="${escapeXML(run.station_name)} 链路测试" style="width:100%;height:auto;font-family:system-ui,sans-serif">
    <title>${escapeXML(run.station_name)} · ${escapeXML(run.target_name)} · ${escapeXML(run.model)}</title>
    <style>.flow{stroke-dasharray:8 5;animation:relay-flow 1.1s linear infinite}@keyframes relay-flow{to{stroke-dashoffset:-26}}@media(prefers-reduced-motion:reduce){.flow{animation:none}}</style>
    <rect width="960" height="${height}" rx="12" fill="#f8fafc"/>
    ${text(30,35,`${run.station_name} · 链路测试`,18)}
    ${text(30,62,`${run.target_name} · ${run.model} · ${run.mode === "stream" ? "流式" : "普通"}`,13)}
    <rect x="30" y="87" width="150" height="48" rx="8" fill="#e0f2fe"/>${text(48,116,"GatewayOps")}
    <path d="M180 111 H300" stroke="#94a3b8" stroke-width="2"/>
    <rect x="300" y="87" width="290" height="48" rx="8" fill="#e0f2fe"/>${text(318,116,run.station_name)}
    <path d="M590 111 H700" stroke="#94a3b8" stroke-width="2"/>
    <rect x="700" y="87" width="230" height="48" rx="8" fill="#e2e8f0"/>${text(716,116,"模型调用结果")}
    ${text(30,163,`成功 ${run.succeeded} · 失败 ${run.failed} · 完成 ${run.results.length}/${run.requested} 次 · 总耗时 ${run.duration_ms} ms`,13)}
    ${rows || text(30,205,"调用未执行")}
  </svg>`
}
