import { createContext } from "react"
import type { SyncSettings, SystemSettings } from "@/lib/api-types"
import type { QueryState } from "@/lib/queries"

export const SystemSettingsContext = createContext<QueryState<SystemSettings> | null>(null)
export const SyncSettingsContext = createContext<QueryState<SyncSettings> | null>(null)
