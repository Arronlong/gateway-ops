type Lease<T> = {promise: Promise<T>; release: () => void}
type Job<T> = {
  key: string; users: number; controller: AbortController
  load: (signal: AbortSignal) => Promise<T>; promise: Promise<T>
  resolve: (value: T) => void; reject: (reason: unknown) => void
}

// 所有渠道共享一个生成队列；缓存只保存静态预览，不保存完整作品 HTML。
export class PelicanResourcePool<T> {
  private cache = new Map<string, {value: T; bytes: number}>()
  private jobs = new Map<string, Job<T>>()
  private queue: Job<T>[] = []
  private active = 0
  private bytes = 0
  private options: {maxBytes: number; maxEntries: number; size: (value: T) => number; timeoutMs?: number}

  constructor(options: {maxBytes: number; maxEntries: number; size: (value: T) => number; timeoutMs?: number}) {this.options = options}
  get cachedBytes() {return this.bytes}
  get cachedEntries() {return this.cache.size}

  acquire(key: string, load: (signal: AbortSignal) => Promise<T>): Lease<T> {
    const cached = this.cache.get(key)
    if (cached) {
      this.cache.delete(key); this.cache.set(key, cached)
      return {promise: Promise.resolve(cached.value), release: () => {}}
    }
    let job = this.jobs.get(key)
    if (!job) {
      let resolve!: (value: T) => void, reject!: (reason: unknown) => void
      const promise = new Promise<T>((done, fail) => {resolve = done; reject = fail})
      job = {key, users: 0, controller: new AbortController(), load, promise, resolve, reject}
      this.jobs.set(key, job); this.queue.push(job)
      queueMicrotask(() => this.pump())
    }
    const current = job
    current.users++
    let released = false
    return {promise: current.promise, release: () => {
      if (released) return
      released = true
      if (--current.users === 0 && this.jobs.get(key) === current) this.cancel(current)
    }}
  }

  delete(key: string) {
    const cached = this.cache.get(key)
    if (cached) {this.bytes -= cached.bytes; this.cache.delete(key)}
    const job = this.jobs.get(key)
    if (job) this.cancel(job)
  }

  clear() {
    this.cache.clear(); this.bytes = 0
    for (const job of this.jobs.values()) this.cancel(job)
    this.queue = []
  }

  private cancel(job: Job<T>, reason = new DOMException("Aborted", "AbortError")) {
    this.jobs.delete(job.key)
    job.controller.abort(reason)
    job.reject(job.controller.signal.reason)
    this.queue = this.queue.filter(item => item !== job)
  }

  private pump() {
    if (this.active) return
    const job = this.queue.shift()
    if (!job) return
    this.active++
    const timeout = setTimeout(() => {
      if (this.jobs.get(job.key) === job) this.cancel(job, new DOMException("预览读取超时，点击作品查看原图", "TimeoutError"))
    }, this.options.timeoutMs ?? 10000)
    Promise.resolve().then(() => {
      job.controller.signal.throwIfAborted()
      return job.load(job.controller.signal)
    }).then(value => {
      if (job.controller.signal.aborted) return
      const bytes = this.options.size(value)
      if (bytes <= this.options.maxBytes) {
        this.cache.set(job.key, {value, bytes}); this.bytes += bytes
        while (this.bytes > this.options.maxBytes || this.cache.size > this.options.maxEntries) {
          const key = this.cache.keys().next().value
          if (key === undefined) break
          const old = this.cache.get(key)!
          this.bytes -= old.bytes; this.cache.delete(key)
        }
      }
      if (this.jobs.get(job.key) === job) this.jobs.delete(job.key)
      job.resolve(value)
    }, error => job.reject(error)).finally(() => {
      clearTimeout(timeout)
      if (this.jobs.get(job.key) === job) this.jobs.delete(job.key)
      this.active--; this.pump()
    })
  }
}
