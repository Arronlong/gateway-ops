import type { PelicanResult, PelicanConfig } from "./api-types"

export function pelicanAutomaticLabel(configs: readonly Pick<PelicanConfig,"enabled">[]) {
 const count=configs.filter(config=>config.enabled).length
 return count===0?"未开启自动测智":count<configs.length?"部分开启自动测智":"已开启自动测智"
}
export function pelicanResultPath(result: Pick<PelicanResult,"id"|"relay_station_id"|"account_external_id"|"group_external_id">,readOnly=false) {
 const base=`${readOnly?"/public":""}/relay-stations/${result.relay_station_id}`
 return `${base}/${result.group_external_id?"groups":"accounts"}/${result.group_external_id||result.account_external_id}/pelican/results/${result.id}`
}
