import { Navigate, Route, Routes } from 'react-router'
import { Shell } from '@/components/layout/shell'
import { LivePage } from '@/pages/live'
import { SettingsPage } from '@/pages/settings'
import { SetupPage } from '@/pages/setup'
import { TunerPage } from '@/pages/tuner'
import { ViewersPage } from '@/pages/viewers'

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<LivePage />} />
        <Route path="viewers" element={<ViewersPage />} />
        <Route path="setup" element={<SetupPage />} />
        <Route path="tuner" element={<TunerPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
