import { useMemo, useState } from 'react'
import { CheckCircle2, CircleDashed, Globe, Radar, TriangleAlert, XCircle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/reui/alert'
import {
  Timeline,
  TimelineContent,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
} from '@/components/reui/timeline'
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
import { Switch } from '@/components/ui/switch'
import { CopyBlock } from '@/components/copy-block'
import { ApiError, api } from '@/lib/api'
import { cnameFields, normalizeDomain } from '@/lib/snippets'
import type { AcmeDomain, DnsCheckResult, DnsCheckStatus } from '@/lib/types'

interface DnsCheckDialogProps {
  domain: AcmeDomain
  onClose: () => void
}

function StepIcon({ status }: { status: DnsCheckStatus }) {
  if (status === 'ok') {
    return <CheckCircle2 className="text-success size-4" />
  }
  if (status === 'failed') {
    return <XCircle className="text-destructive size-4" />
  }
  return <CircleDashed className="text-muted-foreground size-4" />
}

export function DnsCheckDialog({ domain, onClose }: DnsCheckDialogProps) {
  const [target, setTarget] = useState(domain.domain_name ?? '')
  const [skipTxt, setSkipTxt] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<DnsCheckResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  const cname = useMemo(() => cnameFields(domain, target || 'beispiel.de'), [domain, target])
  const zoneLine = `${cname.name}.\tIN\tCNAME\t${cname.target}`

  function reset() {
    setResult(null)
    setError(null)
  }

  async function run() {
    const normalized = normalizeDomain(target)
    if (!normalized || loading) {
      return
    }
    setLoading(true)
    reset()
    try {
      setResult(await api.checkDns(normalized, domain.subdomain, skipTxt))
    } catch (err) {
      setError(
        err instanceof ApiError && err.status === 404
          ? 'Diese Registrierung existiert auf dem Server nicht mehr.'
          : 'Die Prüfung konnte nicht ausgeführt werden.',
      )
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>DNS-Einträge prüfen</DialogTitle>
          <DialogDescription>
            Fragt den CNAME über öffentliche Resolver ab — denselben Weg, den eine
            Zertifizierungsstelle geht.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="check-domain">Domain</Label>
            <div className="relative">
              <Globe className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2" />
              <Input
                id="check-domain"
                className="pr-9"
                placeholder="beispiel.de"
                value={target}
                onChange={(e) => {
                  setTarget(e.target.value)
                  reset()
                }}
                onKeyDown={(e) => e.key === 'Enter' && void run()}
              />
            </div>
          </div>

          <CopyBlock
            label="Erwarteter DNS-Eintrag"
            text={zoneLine}
            copiedLabel="DNS-Eintrag kopiert"
          />

          <div className="flex items-start gap-3 rounded-lg border p-3">
            <Switch
              id="skip-txt"
              checked={skipTxt}
              onCheckedChange={(checked) => {
                setSkipTxt(checked)
                reset()
              }}
            />
            <div className="space-y-1">
              <Label htmlFor="skip-txt" className="text-sm">
                Nur CNAME prüfen, keinen Test-TXT-Wert schreiben
              </Label>
              <p className="text-muted-foreground text-xs">
                Der vollständige Test schreibt kurzzeitig einen Zufallswert in diese Registrierung
                und wartet, bis er öffentlich auflösbar ist. Das dauert bis zu 30 Sekunden.
              </p>
            </div>
          </div>

          {loading && (
            <Alert variant="info">
              <Radar className="animate-pulse" />
              <AlertDescription>Prüfe die Auflösung über öffentliche Resolver …</AlertDescription>
            </Alert>
          )}

          {error && (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {result && (
            <div className="space-y-4">
              <Alert variant={result.valid ? 'success' : 'destructive'}>
                {result.valid ? <CheckCircle2 /> : <TriangleAlert />}
                <AlertDescription>{result.message}</AlertDescription>
              </Alert>

              {/* Highlight the run up to the first step that did not pass. */}
              <Timeline value={result.steps.findIndex((s) => s.status !== 'ok') + 1 || result.steps.length}>
                {result.steps.map((step, index) => (
                  <TimelineItem key={step.id} step={index + 1}>
                    <TimelineIndicator>
                      <StepIcon status={step.status} />
                    </TimelineIndicator>
                    {index < result.steps.length - 1 && <TimelineSeparator />}
                    <TimelineContent>
                      <TimelineTitle className="text-sm">{step.title}</TimelineTitle>
                      <p className="text-muted-foreground font-mono text-xs [overflow-wrap:anywhere]">
                        {step.detail}
                      </p>
                    </TimelineContent>
                  </TimelineItem>
                ))}
              </Timeline>

              {!result.valid && !result.has_cname && (
                <CopyBlock
                  label="Diesen Eintrag anlegen"
                  text={zoneLine}
                  copiedLabel="DNS-Eintrag kopiert"
                />
              )}

              <p className="text-muted-foreground text-xs">
                Abgefragt über: {result.resolvers.join(', ')}
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Schließen
          </Button>
          <Button disabled={!target.trim() || loading} onClick={() => void run()}>
            <Radar className="size-4" />
            {loading ? 'Prüfe …' : result ? 'Erneut prüfen' : 'Prüfen'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
