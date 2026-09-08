import { useState } from 'react'
import { CheckCircle2, CircleDashed, Info, TriangleAlert, XCircle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Badge } from '@/components/reui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import type { MatchResponse, MatchResult } from '@/lib/types'
import { toast } from 'sonner'

interface MatchDomainsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onApplied: () => void
}

function statusBadge(result: MatchResult) {
  switch (result.status) {
    case 'matched':
      return <Badge variant="success-light">Treffer</Badge>
    case 'foreign':
      return <Badge variant="warning-light">fremdes Ziel</Badge>
    case 'error':
      return <Badge variant="destructive-light">Fehler</Badge>
    default:
      return <Badge variant="outline">kein CNAME</Badge>
  }
}

function StatusIcon({ status }: { status: MatchResult['status'] }) {
  if (status === 'matched') {
    return <CheckCircle2 className="text-success mt-0.5 size-4 shrink-0" />
  }
  if (status === 'error') {
    return <XCircle className="text-destructive mt-0.5 size-4 shrink-0" />
  }
  return <CircleDashed className="text-muted-foreground mt-0.5 size-4 shrink-0" />
}

/**
 * Identifies unnamed registrations by resolving _acme-challenge for a list of
 * candidate domains. The CNAME lives in the customer's public zone, so this
 * works even though the server stores nothing that ties a registration to them.
 */
export function MatchDomainsDialog({ open, onOpenChange, onApplied }: MatchDomainsDialogProps) {
  const [input, setInput] = useState('')
  const [apply, setApply] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<MatchResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const candidates = input
    .split(/[\s,;]+/)
    .map((d) => d.trim())
    .filter(Boolean)

  function reset() {
    setResult(null)
    setError(null)
  }

  async function run() {
    if (!candidates.length || loading) {
      return
    }
    setLoading(true)
    reset()
    try {
      const response = await api.matchDomains(candidates, apply)
      setResult(response)
      if (response.applied > 0) {
        toast.success(`${response.applied} Registrierung(en) benannt`)
        onApplied()
      }
    } catch {
      setError('Der Abgleich konnte nicht ausgeführt werden.')
    } finally {
      setLoading(false)
    }
  }

  const matched = result?.results.filter((r) => r.status === 'matched') ?? []
  const others = result?.results.filter((r) => r.status !== 'matched') ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setInput('')
          setApply(false)
          reset()
        }
        onOpenChange(next)
      }}
    >
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Domains zuordnen</DialogTitle>
          <DialogDescription>
            Löst für jede Domain den Eintrag <code>_acme-challenge.&lt;domain&gt;</code> auf und
            gleicht das CNAME-Ziel gegen die vorhandenen Registrierungen ab. Ein Treffer ist
            eindeutig, kein Raten.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="candidates">
              Kandidaten — eine Domain pro Zeile, Komma oder Leerzeichen gehen auch
            </Label>
            <textarea
              id="candidates"
              rows={7}
              className="border-input bg-background focus-visible:ring-ring w-full rounded-md border px-3 py-2 font-mono text-xs focus-visible:ring-2 focus-visible:outline-none"
              placeholder={'kunde-eins.de\nkunde-zwei.de\nshop.kunde-drei.de'}
              value={input}
              onChange={(e) => {
                setInput(e.target.value)
                reset()
              }}
            />
            <p className="text-muted-foreground text-xs">
              {candidates.length} {candidates.length === 1 ? 'Domain' : 'Domains'} · je eine
              DNS-Abfrage, höchstens 500 pro Durchlauf
            </p>
          </div>

          <div className="flex items-start gap-3 rounded-lg border p-3">
            <Switch
              id="apply-names"
              checked={apply}
              onCheckedChange={(checked) => {
                setApply(checked)
                reset()
              }}
            />
            <div className="space-y-1">
              <Label htmlFor="apply-names" className="text-sm">
                Treffer direkt als Namen übernehmen
              </Label>
              <p className="text-muted-foreground text-xs">
                Ohne diesen Schalter wird nur angezeigt, was zusammengehört — nichts wird
                geschrieben.
              </p>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {result && (
            <div className="space-y-3">
              <Alert variant={result.matched > 0 ? 'success' : 'info'}>
                {result.matched > 0 ? <CheckCircle2 /> : <Info />}
                <AlertDescription>
                  {/* One text node: the alert lays its children out in a grid,
                      so nested elements would each land on their own row. */}
                  {`${result.checked} geprüft, ${result.matched} zugeordnet` +
                    (result.applied > 0 ? `, ${result.applied} benannt` : '') +
                    '.'}
                </AlertDescription>
              </Alert>

              {matched.length > 0 && (
                <ul className="divide-y rounded-lg border">
                  {matched.map((r) => (
                    <li key={r.domain} className="flex items-start gap-2.5 p-3">
                      <StatusIcon status={r.status} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="text-sm font-medium">{r.domain}</span>
                          {statusBadge(r)}
                          {r.applied && <Badge variant="info-light">übernommen</Badge>}
                        </div>
                        <p className="text-muted-foreground font-mono text-[11px] [overflow-wrap:anywhere]">
                          {r.subdomain}
                          {r.current_name ? ` · bisher: ${r.current_name}` : ' · bisher ohne Namen'}
                        </p>
                        {r.error && <p className="text-destructive text-xs">{r.error}</p>}
                      </div>
                    </li>
                  ))}
                </ul>
              )}

              {others.length > 0 && (
                <details className="rounded-lg border p-3">
                  <summary className="cursor-pointer text-sm">
                    {others.length} ohne Zuordnung anzeigen
                  </summary>
                  <ul className="mt-2 space-y-1.5">
                    {others.map((r) => (
                      <li key={r.domain} className="flex items-start gap-2.5">
                        <StatusIcon status={r.status} />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm">{r.domain}</span>
                            {statusBadge(r)}
                          </div>
                          {r.target && (
                            <p className="text-muted-foreground font-mono text-[11px] [overflow-wrap:anywhere]">
                              zeigt auf {r.target}
                            </p>
                          )}
                        </div>
                      </li>
                    ))}
                  </ul>
                </details>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Schließen
          </Button>
          <Button disabled={!candidates.length || loading} onClick={() => void run()}>
            {loading ? 'Prüfe …' : apply ? 'Abgleichen und übernehmen' : 'Abgleichen'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
