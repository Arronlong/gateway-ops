import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生测试要求显式扩展名。
import { normalizeBranding, brandingPageTitle, validateBrandingUpload, defaultProductIcon } from "./branding.ts"

test("旧响应和空名称使用默认品牌，标题保留业务上下文", () => {
  assert.deepEqual(normalizeBranding({}), {product_name:"GatewayOps",product_icon:""})
  assert.equal(normalizeBranding({product_name:"  我的产品  "}).product_name,"我的产品")
  assert.equal(normalizeBranding({product_name:"  "}).product_name,"GatewayOps")
  assert.equal(brandingPageTitle("站点 A · 模型价格","我的产品"),"站点 A · 模型价格 · 我的产品")
  assert.match(decodeURIComponent(defaultProductIcon),/M22 12h-4l-3 9L9 3l-3 9H2/)
})

test("只接受大小合规的 PNG、JPEG、WebP 文件", () => {
  for(const type of ["image/png","image/jpeg","image/webp"]) assert.equal(validateBrandingUpload({type,size:1024}),null)
  assert.ok(validateBrandingUpload({type:"image/svg+xml",size:20}))
  assert.ok(validateBrandingUpload({type:"image/png",size:1048577}))
  assert.ok(validateBrandingUpload({type:"image/png",size:0}))
})
