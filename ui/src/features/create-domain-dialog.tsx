import { useState, type FormEvent } from 'react'
import { Globe, TriangleAlert } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { normalizeDomain } from '@/lib/snippets'
import type { AcmeDomain } from '@/lib/types'

/** Matches a hostname such as example.com or sub.example.co.uk. */
const DOMAIN_PATTERN = /^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i

interface CreateDomainDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (domain: AcmeDomain) => void
}

export function CreateDomainDialog({ open, onOpenChange, onCreated }: CreateDomainDialogProps) {
  const [domain, setDomain] = useState('')
  const [touched, setTouched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const invalid = domain.length > 0 && !DOMAIN_PATTERN.test(domain.trim())

  function reset() {
    setDomain('')
    setTouched(false)
    setError(null)
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setTouched(true)
    if (loading || !domain.trim() || invalid) {
      return
    }
    setLoading(true)
    setError(null)
    try {
      const created = await api.createDomain(normalizeDomain(domain))
      reset()
      onOpenChange(false)
      onCreated(created)
    } catch {
      setError('Die Registrierung ist fehlgeschlagen. Bitte das Server-Log prüfen.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Neue Domain anlegen</DialogTitle>
          <DialogDescription>
            Für welche Domain soll ein Zertifikat ausgestellt werden? acme-dns erzeugt daraufhin
            eine eigene Subdomain samt Zugangsdaten, auf die der CNAME zeigt.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-4" noValidate>
          <div className="space-y-2">
            <Label htmlFor="new-domain">Domain</Label>
            <div className="relative">
              <Globe className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2" />
              <Input
                id="new-domain"
                autoFocus
                placeholder="beispiel.de"
                className="pr-9"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                onBlur={() => setTouched(true)}
                aria-invalid={touched && invalid}
              />
            </div>
            {touched && invalid ? (
              <p className="text-destructive text-xs">
                Bitte einen gültigen Hostnamen eingeben, z.&nbsp;B. beispiel.de
              </p>
            ) : (
              <p className="text-muted-foreground text-xs">
                Ein führendes <code className="bg-muted rounded px-1 py-0.5">*.</code> wird
                entfernt — ein Eintrag deckt Domain und Wildcard ab.
              </p>
            )}
          </div>

          {error && (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Abbrechen
            </Button>
            <Button type="submit" disabled={loading || !domain.trim() || invalid}>
              {loading ? 'Wird angelegt …' : 'Anlegen'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
