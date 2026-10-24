import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
// @ts-expect-error Node 22
import { pelicanAutomaticLabel, pelicanResultPath } from "./pelican-sources.ts"

test("渠道自动测智状态区分未开启和部分开启",()=>{
 assert.equal(pelicanAutomaticLabel([]),"未开启自动测智")
 assert.equal(pelicanAutomaticLabel([{enabled:false}]),"未开启自动测智")
 assert.equal(pelicanAutomaticLabel([{enabled:true},{enabled:false}]),"部分开启自动测智")
 assert.equal(pelicanAutomaticLabel([{enabled:true}]),"已开启自动测智")
})
test("账号和分组作品详情使用各自作用域",()=>{
 assert.equal(pelicanResultPath({id:12,relay_station_id:7,account_external_id:20}),"/relay-stations/7/accounts/20/pelican/results/12")
 assert.equal(pelicanResultPath({id:12,relay_station_id:7,account_external_id:0,group_external_id:30}),"/relay-stations/7/groups/30/pelican/results/12")
 assert.equal(pelicanResultPath({id:12,relay_station_id:7,account_external_id:0,group_external_id:30},true),"/public/relay-stations/7/groups/30/pelican/results/12")
})
test("四处页面接入同一套作品组件且监控只读",()=>{
 const source=(path:string)=>readFileSync(new URL(path,import.meta.url),"utf8")
 const pool=source("../app/local-pool-page.tsx"),management=source("../app/relay-stations-page.tsx"),monitor=source("../app/public-relay-monitor-page.tsx"),panel=source("../components/relay/pelican-target-panel.tsx"),provider=source("../components/relay/pelican.tsx")
 assert.match(pool,/usePoolPelicanGallery/)
 assert.match(pool,/data-pelican-pool-account/)
 assert.match(management,/<GroupPelicanPanel stationID=\{stationID\} group=\{group\}/)
 assert.match(monitor,/<PublicGroupPelicanResults/)
 assert.match(panel,/scope="group" readOnly/)
 assert.match(panel,/allowDelete=\{false\}/)
 assert.match(provider,/!context.readOnly\?<PelicanDeleteButton/)
 assert.match(provider,/usePelicanGallery\(galleryQuery\|\|libraryQuery\?null:stationID\)/)
})

test("作品库渠道与号池、分组来源复用原有开关和测一次组件",()=>{
 const channel=readFileSync(new URL("../components/monitor/channel-pelican-panel.tsx",import.meta.url),"utf8")
 const target=readFileSync(new URL("../components/relay/pelican-target-panel.tsx",import.meta.url),"utf8")
 assert.match(channel,/compact&&targets\.some\(target=>target\.station_type!=="newapi"\)\?<div className="ml-auto flex flex-wrap items-center gap-2">/)
 assert.match(channel,/data-pelican-channel-account=\{target\.account_external_id\}/)
 assert.match(channel,/controlsOnly compactControls/)
 assert.doesNotMatch(channel,/compact&&targets\.length\?targets\.filter/)
 assert.match(channel,/compact\?null:<div className="ml-auto"><PelicanRunButton/)
 assert.match(target,/compact\?<PelicanAccountPanel account=\{\{external_id:externalID,name,relay_station_id:stationID\}\} controlsOnly compactControls/)
 assert.match(target,/\{compact\?null:<PelicanAccountPanel account=\{\{external_id:externalID,name,relay_station_id:stationID\}\} controlsOnly\/>\}/)
 const panel=readFileSync(new URL("../components/relay/pelican.tsx",import.meta.url),"utf8")
 const controls=panel.slice(panel.indexOf("export function PelicanAccountPanel"),panel.indexOf("function usePelicanBody"))
 assert.match(controls,/compactControls=false/)
 assert.match(controls,/if\(compactControls\)return/)
 assert.ok(controls.indexOf("<Switch")<controls.indexOf("<Button"))
 const library=readFileSync(new URL("../app/pelican-library-page.tsx",import.meta.url),"utf8")
 assert.match(library,/group\.targets\.every\(target=>target\.config\.enabled\)/)
 assert.match(library,/group\.config\.enabled\)return false/)
})
