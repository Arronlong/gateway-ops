export const GITHUB_REPOSITORY_URL = "https://github.com/Arronlong/gateway-ops"
export const GITHUB_LICENSE_URL = `${GITHUB_REPOSITORY_URL}/blob/main/LICENSE`

export interface VersionInfo {
  name: string
  version: string
  revision: string
  build_time: string
  repository: string
  channel: "stable" | "prerelease" | "development"
}

export interface VersionCheckResult {
  current_version: string
  latest_version?: string
  update_available: boolean | null
  status: "update_available" | "up_to_date" | "current_newer" | "no_release" | "check_failed"
  release_url?: string
  published_at?: string
  checked_at: string
  error?: string
}

export function versionCheckMessage(result: Pick<VersionCheckResult, "status" | "latest_version"> | null) {
  if (!result) return "尚未检查更新"
  switch (result.status) {
    case "update_available": return `发现新版本 v${result.latest_version}`
    case "up_to_date": return "当前已是最新稳定版"
    case "current_newer": return "当前版本高于最新稳定版"
    case "no_release": return "尚未发布正式版本"
    default: return "检查更新失败，请稍后重试"
  }
}
