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
    await assert.rejects(loadPelicanThumbnail("/public/relay-stations/1/groups/2/pelican/results/3", "v1", new AbortController().signal, async()=>{generated++;return {image:new Blob(["jpeg"],{type:"image/jpeg"}),html:"<svg/>"}},false))
    assert.equal(generated,0)
    assert.deepEqual(methods,["GET"])
    await assert.rejects(loadPelicanThumbnail("/channels/1/pelican/results/2", "v1", new AbortController().signal, async()=>({image:new Blob([new Uint8Array(96*1024+1)],{type:"image/jpeg"}),html:"<svg/>"}),true),/96/)
  } finally {globalThis.fetch=original}
})
