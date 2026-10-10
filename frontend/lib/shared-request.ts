export interface SharedRequest<T> {
  promise: Promise<T>
  release: () => void
}

export interface SharedRequestOptions {
  cancelOnNavigation?: boolean
}

interface Entry<T> {
  controller: AbortController
  promise: Promise<T>
  subscribers: number
  navigationReleases: Set<() => void>
  settled: boolean
  aborted: boolean
}

export function createSharedRequestRegistry<T>() {
  const entries = new Map<string, Entry<T>>()

  const scheduleAbortWhenUnused = (entry: Entry<T>) => {
    if (entry.subscribers !== 0 || entry.settled || entry.aborted) return
    queueMicrotask(() => {
      if (entry.subscribers !== 0 || entry.settled || entry.aborted) return
      entry.aborted = true
      entry.controller.abort()
    })
  }

  return {
    acquire(
      key: string,
      run: (signal: AbortSignal) => Promise<T>,
      options: SharedRequestOptions = {},
    ): SharedRequest<T> {
      let entry = entries.get(key)
      if (!entry || entry.aborted || entry.settled) {
        const controller = new AbortController()
        let promise: Promise<T>
        try {
          promise = run(controller.signal)
        } catch (error) {
          promise = Promise.reject(error)
        }
        const nextEntry = {
          controller,
          promise,
          subscribers: 0,
          navigationReleases: new Set<() => void>(),
          settled: false,
          aborted: false,
        } as Entry<T>
        entry = nextEntry
        entries.set(key, nextEntry)
        void nextEntry.promise.then(
          () => {
            nextEntry.settled = true
            nextEntry.navigationReleases.clear()
            if (entries.get(key) === nextEntry) entries.delete(key)
          },
          () => {
            nextEntry.settled = true
            nextEntry.navigationReleases.clear()
            if (entries.get(key) === nextEntry) entries.delete(key)
          },
        )
      }

      entry.subscribers += 1
      let released = false
      const release = () => {
        if (released) return
        released = true
        entry!.subscribers -= 1
        entry!.navigationReleases.delete(release)
        scheduleAbortWhenUnused(entry!)
      }
      if (options.cancelOnNavigation !== false) {
        entry.navigationReleases.add(release)
      }

      return {
        promise: entry.promise,
        release,
      }
    },
    cancelAll() {
      for (const entry of entries.values()) {
        for (const release of [...entry.navigationReleases]) release()
      }
    },
  }
}
