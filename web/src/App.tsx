import { Navigate, Route, Routes, useLocation } from 'react-router'
import { Shell } from '@/components/layout/shell'
import { ActivityPage } from '@/pages/activity'
import { LivePage } from '@/pages/live'
import { SettingsPage } from '@/pages/settings'
import { TunerPage } from '@/pages/tuner'
import { TunersPage } from '@/pages/tuners'
import { ViewerPage } from '@/pages/viewer'
import { ViewersPage } from '@/pages/viewers'

/** The log was a page of its own before it was a tab of the settings: an old link still arrives, with what it searched for. */
function LogsRedirect() {
  const { search } = useLocation()
  return <Navigate to={`/settings/logs${search}`} replace />
}

/** The tuners' pages were at /tuner and /tuner/<name>: an old link or bookmark goes to the same page under /tuners. */
function TunersRedirect() {
  const { pathname, search, hash } = useLocation()
  return <Navigate to={`${pathname.replace(/^\/tuner(?=\/|$)/, '/tuners')}${search}${hash}`} replace />
}

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<LivePage />} />
        <Route path="viewers" element={<ViewersPage />} />
        <Route path="viewers/:name" element={<ViewerPage />} />
        {/* The Connect page is now part of adding a viewer; keep old links working. */}
        <Route path="setup" element={<Navigate to="/viewers" replace />} />
        <Route path="tuners" element={<TunersPage />} />
        <Route path="tuners/:name" element={<TunerPage />} />
        <Route path="tuners/:name/:tab" element={<TunerPage />} />
        <Route path="tuner" element={<TunersRedirect />} />
        <Route path="tuner/*" element={<TunersRedirect />} />
        <Route path="activity" element={<ActivityPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="settings/:tab" element={<SettingsPage />} />
        <Route path="logs" element={<LogsRedirect />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
