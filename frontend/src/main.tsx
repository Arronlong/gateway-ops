import { lazy, StrictMode, Suspense, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { ThemeProvider } from '@/components/theme-provider'
import { AuthProvider } from '@/lib/auth-context'
import { RefreshProvider } from '@/lib/refresh-context'
import { cancelInFlightQueries } from '@/lib/queries'
import { installNavigationCancellation } from '@/lib/navigation-cancellation'
import { AddChannelProvider } from '@/lib/add-channel-context'
import { AuthGate } from '@/components/auth/auth-gate'
import { AppShell } from '@/components/app-shell'
import { SettingsProvider } from '@/components/settings-provider'
import { ProductBrandingProvider } from '@/components/product-branding'
import { Toaster } from '@/components/ui/sonner'
import { PageErrorBoundary } from '@/components/ui/page-error-boundary'
const DashboardPage = lazy(() => import('@/app/page'))
const ChannelsPage = lazy(() => import('@/app/channels-page'))
const CaptchaPage = lazy(() => import('@/app/captcha-page'))
const NotificationsPage = lazy(() => import('@/app/notifications-page'))
const SystemSettingsPage = lazy(() => import('@/app/system-settings-page'))
const RelayStationsPage = lazy(() => import('@/app/relay-stations-page'))
const OperationsCostsPage = lazy(() => import('@/app/operations-costs-page'))
const LocalPoolPage = lazy(() => import('@/app/local-pool-page'))
const PelicanLibraryPage = lazy(() => import('@/app/pelican-library-page'))
const PublicRelayMonitorPage = lazy(() => import('@/app/public-relay-monitor-page'))
const PublicModelPricingPage = lazy(() => import('@/app/public-model-pricing-page'))
const PublicServiceStatusPage = lazy(() => import('@/app/public-service-status-page'))
import '@/app/globals.css'

function ProtectedApplication() {
  useEffect(() => installNavigationCancellation(cancelInFlightQueries), [])

  return (
    <AuthProvider>
      <AuthGate>
        <RefreshProvider>
          <SettingsProvider>
            <AddChannelProvider>
              <AppShell />
            </AddChannelProvider>
          </SettingsProvider>
        </RefreshProvider>
      </AuthGate>
    </AuthProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <PageErrorBoundary>
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem disableTransitionOnChange>
      <BrowserRouter useTransitions={false}>
        <ProductBrandingProvider>
        <Suspense fallback={<p role="status" className="p-6 text-sm text-muted-foreground">{'\u6b63\u5728\u52a0\u8f7d\u9875\u9762\u2026'}</p>}><Routes>
          <Route path="public/relay-monitor" element={<PublicRelayMonitorPage />} />
          <Route path="public/relay-monitor/:stationID" element={<PublicRelayMonitorPage />} />
          <Route path="public/model-pricing/:stationID" element={<PublicModelPricingPage />} />
          <Route path="public/service-status" element={<PublicServiceStatusPage />} />
          <Route element={<ProtectedApplication />}>
            <Route index element={<DashboardPage />} />
            <Route path="channels" element={<ChannelsPage />} />
            <Route path="captcha" element={<CaptchaPage />} />
            <Route path="notifications" element={<NotificationsPage />} />
            <Route path="alerts" element={<NotificationsPage />} />
            <Route path="settings" element={<SystemSettingsPage />} />
            <Route path="relay-stations" element={<RelayStationsPage />} />
            <Route path="pelican" element={<PelicanLibraryPage />} />
            <Route path="operations/costs" element={<OperationsCostsPage />} />
            <Route path="operations/local-pool" element={<LocalPoolPage />} />
          </Route>
        </Routes></Suspense>
        </ProductBrandingProvider>
      </BrowserRouter>
      <Toaster richColors closeButton position="top-right" />
    </ThemeProvider>
    </PageErrorBoundary>
  </StrictMode>,
)
