import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Server, TriangleAlert } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError, api, session } from '@/lib/api'

function messageFor(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return 'Der Server ist nicht erreichbar.'
  }
  if (error.status === 429 || error.code === 'too_many_attempts') {
    return 'Zu viele Fehlversuche. Bitte in 15 Minuten erneut versuchen.'
  }
  if (error.status === 401) {
    return 'Benutzername oder Passwort ist falsch.'
  }
  if (error.status === 404) {
    return 'Die Verwaltung ist auf diesem Server nicht aktiviert (auth.admin_user fehlt in der config.cfg).'
  }
  return 'Anmeldung fehlgeschlagen.'
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (loading || !username || !password) {
      return
    }
    setLoading(true)
    setError(null)
    try {
      session.store(await api.login(username, password))
      navigate('/domains', { replace: true })
    } catch (err) {
      setError(messageFor(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="from-background via-background to-primary/5 flex min-h-screen items-center justify-center bg-gradient-to-b p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="flex items-center gap-3">
            <span className="bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-lg">
              <Server className="size-5" />
            </span>
            <div>
              <CardTitle className="text-lg">acme-dns</CardTitle>
              <CardDescription>Verwaltung der DNS-01-Delegierungen</CardDescription>
            </div>
          </div>
        </CardHeader>

        <CardContent>
          <form onSubmit={submit} className="space-y-4" noValidate>
            <div className="space-y-2">
              <Label htmlFor="username">Benutzername</Label>
              <Input
                id="username"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Passwort</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>

            {error && (
              <Alert variant="destructive">
                <TriangleAlert />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <Button type="submit" className="w-full" disabled={loading || !username || !password}>
              {loading ? 'Anmelden …' : 'Anmelden'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
