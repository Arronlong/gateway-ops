import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
// @ts-expect-error Node 22
import { pelicanGroupLabels } from "./pelican.ts"

test("\u4ec5\u6e20\u9053\u6d4b\u667a\u5c55\u793a\u5206\u7ec4\u548c\u500d\u7387",()=>{
 const source=(path:string)=>readFileSync(new URL(path,import.meta.url),"utf8")
 const provider=source("../components/relay/pelican.tsx")
 const channel=source("../components/monitor/channel-pelican-panel.tsx")
 const targets=source("../components/relay/pelican-target-panel.tsx")
 assert.match(provider,/showKeyGroup=false/)
 assert.match(provider,/context\.showKeyGroup\s*\?\s*<p data-pelican-key-group/)
 assert.match(channel,/<PelicanProvider[^>]*showKeyGroup/)
 assert.doesNotMatch(targets,/<PelicanProvider[^>]*showKeyGroup/)
})

test("作品分组倍率区分未知和零倍率",()=>{
 assert.deepEqual(pelicanGroupLabels({}),{name:"分组未知",rate:"倍率未知"})
 assert.deepEqual(pelicanGroupLabels({upstream_group_name:"长分组名称",upstream_group_multiplier:0}),{name:"长分组名称",rate:"0×"})
 assert.equal(pelicanGroupLabels({upstream_group_multiplier:0.25}).rate,"0.25×")
 assert.equal(pelicanGroupLabels({upstream_group_multiplier:NaN}).rate,"倍率未知")
 assert.equal(pelicanGroupLabels({upstream_group_multiplier:-1}).rate,"倍率未知")
})

test("图片下方分组和倍率单行左右对齐并保留长名称提示",()=>{
 const source=readFileSync(new URL("../components/relay/pelican.tsx",import.meta.url),"utf8")
 assert.match(source,/data-pelican-key-group/)
 assert.match(source,/min-w-0 flex-1 truncate[^>]*title=\{groupLabels.name\}/)
 assert.match(source,/shrink-0 whitespace-nowrap tabular-nums/)
})
