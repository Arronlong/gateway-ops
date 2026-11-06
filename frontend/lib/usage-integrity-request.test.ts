import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { fetchUsageIntegrity } from "./usage-integrity-request.ts"

test("\u6821\u9a8c\u6d41\u9010\u6761\u63a8\u9001\u9636\u6bb5\u5e76\u8fd4\u56de\u7ed3\u679c",async()=>{
  const original=globalThis.fetch
  const events:string[]=[]
  globalThis.fetch=async(_input,init)=>{
    assert.equal(init?.method,"GET")
    assert.equal(new Headers(init?.headers).get("Accept"),"text/event-stream")
    const body='data: {"stage":"usage","message":"total"}\n\ndata: {"stage":"usage","message":"month"}\n\ndata: {"stage":"done","ok":true,"message":"done","data":{"status":"matched","local_count":2}}\n\n'
    return new Response(new ReadableStream({start(controller){for(const chunk of [body.slice(0,21),body.slice(21,83),body.slice(83)])controller.enqueue(new TextEncoder().encode(chunk));controller.close()}}),{headers:{"Content-Type":"text/event-stream"}})
  }
  try {
    const result=await fetchUsageIntegrity("/channels/1/usage-integrity",new AbortController().signal,event=>events.push(event.message))
    assert.equal(result.status,"matched")
    assert.deepEqual(events,["total","month","done"])
  }finally{globalThis.fetch=original}
})

test("\u6821\u9a8c\u4e2d\u65ad\u548c\u670d\u52a1\u7aef\u5f02\u5e38\u5fc5\u987b\u544a\u77e5",async()=>{
  const original=globalThis.fetch
  try {
    for(const body of ['data: {"stage":"usage","message":"working"}\n\n','data: {"stage":"error","ok":false,"message":"upstream refused"}\n\n']) {
      globalThis.fetch=async()=>new Response(body,{headers:{"Content-Type":"text/event-stream"}})
      await assert.rejects(fetchUsageIntegrity("/test",new AbortController().signal,()=>{}),body.includes("refused")?/upstream refused/:/\u4e2d\u65ad/)
    }
    globalThis.fetch=async()=>new Response(JSON.stringify({data:{status:"matched"}}),{headers:{"Content-Type":"application/json"}})
    assert.equal((await fetchUsageIntegrity("/test",new AbortController().signal,()=>{})).status,"matched")
  }finally{globalThis.fetch=original}
})

test("\u7b49\u5f85\u54cd\u5e94\u5934\u548c JSON \u54cd\u5e94\u4f53\u4e5f\u9700\u8d85\u65f6\u4e2d\u6b62",async()=>{
  const originalFetch=globalThis.fetch,originalTimeout=globalThis.setTimeout
  globalThis.setTimeout=((callback: (...args: unknown[]) => void,delay?: number,...args: unknown[])=>originalTimeout(callback,Math.min(Number(delay),20),...args)) as typeof setTimeout
  try {
    for(const phase of ["headers","json"]) {
      let requestSignal:AbortSignal|undefined
      globalThis.fetch=async(_input,init)=>{
        requestSignal=init?.signal ?? undefined
        if(phase==="headers")return new Promise<Response>(()=>{})
        return new Response(new ReadableStream(),{headers:{"Content-Type":"application/json"}})
      }
      let watchdog:ReturnType<typeof setTimeout>|undefined
      const pending=fetchUsageIntegrity("/test",new AbortController().signal,()=>{})
      try {
        await assert.rejects(Promise.race([pending,new Promise<never>((_,reject)=>{watchdog=originalTimeout(()=>reject(new Error("watchdog")),200)})]),/\u8d85\u65f6/)
        assert.equal(requestSignal?.aborted,true)
      }finally{clearTimeout(watchdog)}
    }
  }finally{globalThis.fetch=originalFetch;globalThis.setTimeout=originalTimeout}
})
