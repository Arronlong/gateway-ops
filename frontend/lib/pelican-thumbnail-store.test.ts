import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { loadPelicanThumbnail } from "./pelican-thumbnail-store.ts"

test("saved thumbnails skip the complete artwork and rendering", async () => {
  const original = globalThis.fetch
  globalThis.fetch = async () => new Response(new Blob(["jpeg"], {type:"image/jpeg"}), {headers:{"Content-Type":"image/jpeg"}})
  try {
    const image = await loadPelicanThumbnail("/channels/1/pelican/results/2", "v1", new AbortController().signal, async () => {throw new Error("saved thumbnail fetched full artwork")}, true)
    assert.equal(await image.text(), "jpeg")
  } finally {globalThis.fetch = original}
})

test("missing thumbnail is generated once and persisted for the next visit", async () => {
  const original = globalThis.fetch
  let saved: Blob | null = null, generated = 0, uploads = 0
  globalThis.fetch = async (_url, options) => {
    if (options?.method === "PUT") {
      uploads++
      assert.match(new Headers(options.headers).get("X-Pelican-Source-SHA256") ?? "", /^[a-f0-9]{64}$/)
      saved = options.body as Blob
      return new Response(null, {status:204})
    }
    return saved ? new Response(saved, {headers:{"Content-Type":"image/jpeg"}}) : new Response(null,{status:404})
  }
  try {
    const create = async () => {generated++; return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}}
    for (let i=0;i<2;i++) await loadPelicanThumbnail("/channels/1/pelican/results/2", "v1", new AbortController().signal, create, true)
    assert.equal(generated,1)
    assert.equal(uploads,1)
  } finally {globalThis.fetch=original}
})

test("public visitors never upload and oversized images are rejected", async () => {
  const original=globalThis.fetch
  const methods:string[]=[]
  globalThis.fetch=async (_url,options)=>{methods.push(options?.method??"GET");return new Response(null,{status:404})}
  try {
    let generated = 0
    const preview=await loadPelicanThumbnail("/public/relay-stations/1/groups/2/pelican/results/3", "v1", new AbortController().signal, async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},false)
    assert.equal(await preview.text(),"jpeg")
    assert.equal(generated,1)
    assert.deepEqual(methods,["GET"])
    await assert.rejects(loadPelicanThumbnail("/channels/1/pelican/results/2", "v1", new AbortController().signal, async()=>({image:new Blob([new Uint8Array(96*1024+1)],{type:"image/jpeg"}),html:"<svg/>"}),true),/96/)
  } finally {globalThis.fetch=original}
})

test("\u5df2\u4fdd\u5b58\u7684\u767d\u56fe\u4f1a\u91cd\u65b0\u751f\u6210\u5e76\u8986\u76d6", async () => {
  const original=globalThis.fetch
  const methods:string[]=[]
  let saved:Blob|null=null
  globalThis.fetch=async (_url,options)=>{
    methods.push(options?.method??"GET")
    if(options?.method==="PUT"){saved=options.body as Blob;return new Response(null,{status:204})}
    return new Response(saved??new Blob(["blank"],{type:"image/jpeg"}))
  }
  try {
    const preview=await loadPelicanThumbnail("/channels/1/pelican/results/2","v1",new AbortController().signal,
      async()=>({image:new Blob(["artwork"],{type:"image/jpeg"}),html:"<svg/>"}),true,
      async(image:Blob)=>{if(await image.text()==="blank")throw new Error("blank thumbnail")})
    assert.equal(await preview.text(),"artwork")
    assert.deepEqual(methods,["GET","PUT","GET"])
  } finally {globalThis.fetch=original}
})

test("\u6743\u9650\u548c\u670d\u52a1\u5668\u9519\u8bef\u4e0d\u89e6\u53d1\u539f\u4f5c\u54c1\u751f\u6210", async () => {
  const original=globalThis.fetch
  let generated=0
  try {
    for(const status of [403,500]) {
      globalThis.fetch=async()=>new Response(null,{status})
      await assert.rejects(loadPelicanThumbnail("/channels/1/pelican/results/2","v1",new AbortController().signal,
        async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},false),new RegExp(String(status)))
    }
    assert.equal(generated,0)
  } finally {globalThis.fetch=original}
})

test("\u56fe\u7247\u68c0\u67e5\u4e2d\u53d6\u6d88\u540e\u4e0d\u518d\u751f\u6210\u6216\u4e0a\u4f20", async () => {
  const original=globalThis.fetch
  let generated=0,requests=0
  const controller=new AbortController()
  globalThis.fetch=async()=>{requests++;return new Response(new Blob(["jpeg"],{type:"image/jpeg"}))}
  try {
    await assert.rejects(loadPelicanThumbnail("/channels/1/pelican/results/2","v1",controller.signal,
      async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},true,
      async()=>{controller.abort();controller.signal.throwIfAborted()}),{name:"AbortError"})
    assert.equal(generated,0)
    assert.equal(requests,1)
  } finally {globalThis.fetch=original}
})

test("\u4e0a\u4f20\u6682\u65f6\u5931\u8d25\u4f1a\u91cd\u4f20\u539f\u56fe\u5e76\u56de\u8bfb\u786e\u8ba4", async () => {
  const original=globalThis.fetch
  let uploads=0,generated=0,reads=0,saved:Blob|null=null
  globalThis.fetch=async(_url,options)=>{
    if(options?.method==="PUT") {
      if(++uploads===1)return new Response(JSON.stringify({error:"storage temporarily unavailable"}),{status:503})
      saved=options.body as Blob
      return new Response(null,{status:204})
    }
    reads++
    return saved?new Response(saved):new Response(null,{status:404})
  }
  try {
    const preview=await loadPelicanThumbnail("/channels/1/pelican/results/2","v1",new AbortController().signal,
      async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},true)
    assert.equal(await preview.text(),"jpeg")
    assert.equal(uploads,2)
    assert.equal(generated,1)
    assert.equal(reads,2)
  } finally {globalThis.fetch=original}
})

test("\u670d\u52a1\u7aef\u6ca1\u6709\u771f\u6b63\u4fdd\u5b58\u7684\u7f29\u7565\u56fe\u4f1a\u62a5\u544a\u4fdd\u5b58\u5931\u8d25", async () => {
  const original=globalThis.fetch
  let uploads=0
  const failures:Error[]=[]
  globalThis.fetch=async(_url,options)=>{
    if(options?.method==="PUT"){uploads++;return new Response(null,{status:204})}
    return new Response(JSON.stringify({error:"thumbnail not found"}),{status:404})
  }
  try {
    const preview=await loadPelicanThumbnail("/channels/1/pelican/results/2","v1",new AbortController().signal,
      async()=>({image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}),true,undefined,(error:Error)=>failures.push(error))
    assert.equal(await preview.text(),"jpeg")
    assert.equal(uploads,2)
    assert.equal(failures.length,1)
    assert.match(failures[0].message,/404.*thumbnail not found/)
  } finally {globalThis.fetch=original}
})

test("\u7f3a\u5c11\u767b\u5f55\u4ee4\u724c\u7684\u539f\u59cb\u539f\u56e0\u4e0d\u88ab\u5f53\u6210\u7f3a\u56fe", async () => {
  const original=globalThis.fetch
  let generated=0
  globalThis.fetch=async()=>new Response(JSON.stringify({error:"missing token"}),{status:401})
  try {
    await assert.rejects(loadPelicanThumbnail("/channels/1/pelican/results/2","v1",new AbortController().signal,
      async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},true),/401.*missing token/)
    assert.equal(generated,0)
  } finally {globalThis.fetch=original}
})
