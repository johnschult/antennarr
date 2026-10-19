import { Navigate, Route, Routes } from 'react-router'
import { Shell } from '@/components/layout/shell'
import { ActivityPage } from '@/pages/activity'
import { LivePage } from '@/pages/live'
import { SettingsPage } from '@/pages/settings'
import { TunerPage } from '@/pages/tuner'
import { ViewersPage } from '@/pages/viewers'

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<LivePage />} />
        <Route path="viewers" element={<ViewersPage />} />
        {/* The Connect page is now part of adding a viewer; keep old links working. */}
        <Route path="setup" element={<Navigate to="/viewers" replace />} />
        <Route path="tuner" element={<TunerPage />} />
        <Route path="activity" element={<ActivityPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
