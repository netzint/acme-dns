import { useEffect } from 'react'
import { Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { Toaster } from '@/components/ui/sonner'
import { session, setUnauthorizedHandler } from '@/lib/api'
import LoginPage from '@/pages/login'
import DomainsPage from '@/pages/domains'

function RequireSession({ children }: { children: React.ReactNode }) {
  if (!session.token) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

export default function App() {
  const navigate = useNavigate()

  // Management sessions live in the server's memory, so a restart invalidates a
  // stored token. Turn the resulting 401 into a trip back to the login screen.
  useEffect(() => {
    setUnauthorizedHandler(() => navigate('/login', { replace: true }))
  }, [navigate])

  return (
    <>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/domains"
          element={
            <RequireSession>
              <DomainsPage />
            </RequireSession>
          }
        />
        <Route path="*" element={<Navigate to="/domains" replace />} />
      </Routes>
      <Toaster position="bottom-right" />
    </>
  )
}
