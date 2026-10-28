import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8")

test("渠道卡片直接展示最近三份结果，不再弹出独立配置面板", () => {
  const cards = source("../components/monitor/channel-cards.tsx")
  const panel = source("../components/monitor/channel-pelican-panel.tsx")
  assert.match(cards, /<ChannelPelicanPanel channel=\{c\}/)
  assert.doesNotMatch(cards, /ChannelPelicanDialog|setPelicanChannel/)
  assert.match(cards, /limit=\{3\}/)
  assert.match(panel, /\/pelican\?channel_id=/)
  assert.match(panel, /<PelicanAccountPanel/)
})

test("独立作品页按渠道逐行展示且不再按作品分页或平铺", () => {
  const page = source("../app/pelican-library-page.tsx")
  assert.match(page, /useChannelPelicanGallery/)
  assert.match(page, /data-pelican-channel/)
  assert.match(page, /<ChannelPelicanPanel/)
  assert.match(page, /historyCount/)
  assert.doesNotMatch(page, /PelicanLibraryResults|page_size|grid-cols-[234]/)
})

test("独立页使用精简渠道标题和绘制入口，不重复配置面板", () => {
  const page=source("../app/pelican-library-page.tsx")
  const panel=source("../components/monitor/channel-pelican-panel.tsx")
  assert.match(page, /compact/)
  assert.doesNotMatch(page, /channelTypeLabel|group\.targets\.length.*个关联账号/)
  assert.match(panel, /<PelicanRunButton/)
  assert.match(panel, /channelID=\{channel\.id\}/)
  assert.match(panel, /compact&&targets\.length\?/)
  assert.doesNotMatch(panel, /station_type==="newapi".*暂不支持读取/)
})

test("测智作品页渠道名称链接到该渠道的上游网站", () => {
  const panel=source("../components/monitor/channel-pelican-panel.tsx")
  assert.match(panel, /Pick<Channel,"id"\|"name"\|"site_url">/)
  assert.match(panel, /href=\{channel\.site_url\}[^>]*target="_blank"[^>]*rel="noopener noreferrer"/)
  assert.match(panel, /aria-label=\{`打开上游网站：\$\{channel\.name\}`\}/)
})

test("作品库展示数量跟随系统配置的保留数量", () => {
  const panel=source("../components/monitor/channel-pelican-panel.tsx")
  const page=source("../app/pelican-library-page.tsx")
  assert.match(panel, /settings\.data\?\.pelican_history_count\?\?30/)
  assert.match(panel, /<PelicanResultsStrip limit=\{displayLimit\} allowDelete=\{compact\} grid=\{!compact\}\/\>/)
  assert.match(page, /每组最多 \{historyCount\} 份结果/)
})

test("模型和思考强度筛选位于每个渠道标题行且各自独立", () => {
  const page=source("../app/pelican-library-page.tsx")
  const panel=source("../components/monitor/channel-pelican-panel.tsx")
  assert.doesNotMatch(page, /modelFilter|effortFilter|filterOptions|按模型筛选|按思考强度筛选/)
  assert.match(panel, /const \[modelFilter,setModelFilter\]=useState\("all"\)/)
  assert.match(panel, /const \[effortFilter,setEffortFilter\]=useState\("all"\)/)
  const header=panel.slice(panel.indexOf("data-pelican-group-header"),panel.indexOf("</header>"))
  assert.match(header, /filterOptions\.models\.map/)
  assert.match(header, /filterOptions\.efforts\.map/)
  assert.match(header, /aria-pressed=\{modelFilter===model\}/)
  assert.match(header, /aria-pressed=\{effortFilter===effort\}/)
  assert.match(panel, /getPelicanFilterOptions\(group\?\[group\]:\[\]\)/)
  assert.match(panel, /matchesPelicanFilters\(result,modelFilter,effortFilter\)/)
})
