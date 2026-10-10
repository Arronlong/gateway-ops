import type { ReactNode } from "react"
import { useSyncSettingsQuery, useSystemSettingsQuery } from "@/lib/queries"
import { SyncSettingsContext, SystemSettingsContext } from "@/lib/settings-context"

export function SettingsProvider({ children }: { children: ReactNode }) {
  const systemSettings = useSystemSettingsQuery()
  const syncSettings = useSyncSettingsQuery()

  return (
    <SystemSettingsContext.Provider value={systemSettings}>
      <SyncSettingsContext.Provider value={syncSettings}>
        {children}
      </SyncSettingsContext.Provider>
    </SystemSettingsContext.Provider>
  )
}
