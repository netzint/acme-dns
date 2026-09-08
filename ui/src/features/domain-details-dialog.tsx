import { useMemo, useState } from 'react'
import { Eye, EyeOff, Info, KeyRound, Radar, Save } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Badge } from '@/components/reui/badge'
import {
  Stepper,
  StepperContent,
  StepperIndicator,
  StepperItem,
  StepperNav,
  StepperPanel,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/reui/stepper'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CopyBlock } from '@/components/copy-block'
import { CopyValue } from '@/components/copy-value'
import { ConfirmDialog } from '@/features/confirm-dialog'
import { api } from '@/lib/api'
import {
  certbotSnippet,
  cnameFields,
  curlSnippet,
  legoStorageJson,
  normalizeDomain,
  traefikCompose,
  traefikStatic,
} from '@/lib/snippets'
import type { AcmeDomain, ServerInfo } from '@/lib/types'
import { toast } from 'sonner'

interface DomainDetailsDialogProps {
  domain: AcmeDomain
  server: ServerInfo
  /** True right after registration, which changes the wording to onboarding. */
  isNew: boolean
  onClose: (action: 'check' | 'done', domain: AcmeDomain) => void
}

const STEPS = [
  { step: 1, title: 'DNS-Eintrag' },
  { step: 2, title: 'Zugangsdaten' },
  { step: 3, title: 'Client' },
  { step: 4, title: 'Prüfen' },
]

export function DomainDetailsDialog({ domain, server, isNew, onClose }: DomainDetailsDialogProps) {
  const [record, setRecord] = useState(domain)
  const [target, setTarget] = useState(domain.domain_name ?? '')
  const [currentStep, setCurrentStep] = useState(1)
  const [showPassword, setShowPassword] = useState(isNew)
  const [busy, setBusy] = useState(false)
  const [rotateOpen, setRotateOpen] = useState(false)

  const effectiveTarget = target || 'beispiel.de'
  const nameDirty = normalizeDomain(target) !== normalizeDomain(record.domain_name ?? '')

  const cname = useMemo(() => cnameFields(record, effectiveTarget), [record, effectiveTarget])
  const zoneLine = `${cname.name}.\tIN\tCNAME\t${cname.target}`

  const maskedPassword = record.password ? '•'.repeat(Math.min(record.password.length, 40)) : ''

  async function saveName() {
    if (!nameDirty || busy) {
      return
    }
    setBusy(true)
    try {
      const updated = await api.renameDomain(record.subdomain, normalizeDomain(target))
      // The rename response carries the stored copy; keep the password we already have.
      setRecord({ ...updated, password: record.password })
      toast.success('Domain gespeichert')
    } catch {
      toast.error('Speichern fehlgeschlagen')
    } finally {
      setBusy(false)
    }
  }

  async function rotate() {
    setRotateOpen(false)
    setBusy(true)
    try {
      setRecord(await api.rotateCredentials(record.subdomain))
      setShowPassword(true)
      toast.success('Neue Zugangsdaten erzeugt')
    } catch {
      toast.error('Erzeugen fehlgeschlagen')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Dialog open onOpenChange={(open) => !open && onClose('done', record)}>
        <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2.5">
              {record.domain_name || record.subdomain}
              {isNew && <Badge variant="success-light">neu angelegt</Badge>}
            </DialogTitle>
            <DialogDescription>
              {isNew
                ? 'Die Registrierung steht. Es fehlen noch zwei Handgriffe: der CNAME in der Zone der Domain und die Zugangsdaten im Client.'
                : 'Alle Werte und fertigen Konfigurationsblöcke für diese Registrierung.'}
            </DialogDescription>
          </DialogHeader>

          <Stepper value={currentStep} onValueChange={setCurrentStep} className="space-y-6">
            <StepperNav>
              {STEPS.map(({ step, title }) => (
                <StepperItem key={step} step={step} className="not-last:flex-1">
                  <StepperTrigger className="gap-2.5">
                    <StepperIndicator>{step}</StepperIndicator>
                    <StepperTitle className="hidden text-sm sm:block">{title}</StepperTitle>
                  </StepperTrigger>
                  {step < STEPS.length && <StepperSeparator />}
                </StepperItem>
              ))}
            </StepperNav>

            <StepperPanel>
              {/* 1 — the record the domain owner has to create */}
              <StepperContent value={1} className="space-y-4">
                <p className="text-muted-foreground text-sm">
                  Einmalig in der Zone von <strong>{effectiveTarget}</strong> anlegen. Danach läuft
                  jede Ausstellung und Verlängerung ohne weitere DNS-Änderung.
                </p>

                <dl className="grid gap-2.5">
                  <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Typ
                    </dt>
                    <dd className="font-mono text-xs">CNAME</dd>
                  </div>
                  <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Name
                    </dt>
                    <dd className="min-w-0">
                      <CopyValue value={cname.name} copiedLabel="Name kopiert" />
                    </dd>
                  </div>
                  <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Ziel
                    </dt>
                    <dd className="min-w-0">
                      <CopyValue value={cname.target} copiedLabel="Ziel kopiert" />
                    </dd>
                  </div>
                </dl>

                <CopyBlock
                  label="Als Zonendatei-Zeile"
                  text={zoneLine}
                  copiedLabel="DNS-Eintrag kopiert"
                />
              </StepperContent>

              {/* 2 — the credentials the ACME client needs */}
              <StepperContent value={2} className="space-y-4">
                <p className="text-muted-foreground text-sm">
                  Damit schreibt der ACME-Client den TXT-Wert in diese Subdomain.
                </p>

                <dl className="grid gap-2.5">
                  <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Benutzer
                    </dt>
                    <dd className="min-w-0">
                      <CopyValue value={record.username} copiedLabel="Benutzer kopiert" />
                    </dd>
                  </div>
                  <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Passwort
                    </dt>
                    <dd className="flex min-w-0 items-center gap-1">
                      {record.credentials_available ? (
                        <>
                          <CopyValue
                            value={record.password}
                            display={showPassword ? record.password : maskedPassword}
                            copiedLabel="Passwort kopiert"
                          />
                          <Button
                            size="icon"
                            variant="ghost"
                            className="size-8 shrink-0"
                            title={showPassword ? 'Verbergen' : 'Anzeigen'}
                            onClick={() => setShowPassword(!showPassword)}
                          >
                            {showPassword ? (
                              <EyeOff className="size-4" />
                            ) : (
                              <Eye className="size-4" />
                            )}
                          </Button>
                        </>
                      ) : (
                        <Badge variant="warning-light">nicht gespeichert</Badge>
                      )}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
                    <dt className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
                      Subdomain
                    </dt>
                    <dd className="min-w-0">
                      <CopyValue value={record.subdomain} copiedLabel="Subdomain kopiert" />
                    </dd>
                  </div>
                </dl>

                {!record.credentials_available && (
                  <Alert variant="warning">
                    <Info />
                    <AlertDescription>
                      Für diese Registrierung liegt kein wiederherstellbares Passwort vor — sie
                      stammt aus der Zeit vor der Passwortspeicherung. Über „Neue Zugangsdaten"
                      lässt sich ein frisches Passwort erzeugen; der CNAME aus Schritt 1 bleibt
                      unverändert gültig.
                    </AlertDescription>
                  </Alert>
                )}

                <Button variant="outline" disabled={busy} onClick={() => setRotateOpen(true)}>
                  <KeyRound className="size-4" />
                  Neue Zugangsdaten
                </Button>
              </StepperContent>

              {/* 3 — ready to paste client configuration */}
              <StepperContent value={3} className="space-y-4">
                <p className="text-muted-foreground text-sm">
                  Fertige Blöcke zum Einfügen — der Reiter passend zum eingesetzten Client.
                </p>

                <Tabs defaultValue="traefik">
                  <TabsList>
                    <TabsTrigger value="traefik">Traefik</TabsTrigger>
                    <TabsTrigger value="lego">lego / acme-dns.json</TabsTrigger>
                    <TabsTrigger value="certbot">certbot</TabsTrigger>
                    <TabsTrigger value="curl">curl</TabsTrigger>
                  </TabsList>

                  <TabsContent value="traefik" className="space-y-4 pt-4">
                    <CopyBlock
                      text={traefikCompose(record, effectiveTarget, server)}
                      copiedLabel="Traefik-Konfiguration kopiert"
                    />
                    <CopyBlock
                      label="Alternativ: statische Konfiguration"
                      text={traefikStatic(record, effectiveTarget, server)}
                      copiedLabel="Traefik-Konfiguration kopiert"
                    />
                  </TabsContent>

                  <TabsContent value="lego" className="space-y-3 pt-4">
                    <p className="text-muted-foreground text-xs">
                      Inhalt der Datei aus{' '}
                      <code className="bg-muted rounded px-1 py-0.5">ACME_DNS_STORAGE_PATH</code>.
                      Existiert die Datei schon, wird dieser Block als weiterer Schlüssel in das
                      vorhandene JSON-Objekt eingefügt.
                    </p>
                    <CopyBlock
                      text={legoStorageJson(record, effectiveTarget, server)}
                      copiedLabel="acme-dns.json kopiert"
                    />
                  </TabsContent>

                  <TabsContent value="certbot" className="pt-4">
                    <CopyBlock
                      text={certbotSnippet(record, effectiveTarget, server)}
                      copiedLabel="certbot-Anleitung kopiert"
                    />
                  </TabsContent>

                  <TabsContent value="curl" className="space-y-3 pt-4">
                    <p className="text-muted-foreground text-xs">
                      Zum manuellen Testen, ob die Zugangsdaten greifen.
                    </p>
                    <CopyBlock
                      text={curlSnippet(record, server)}
                      copiedLabel="curl-Befehl kopiert"
                    />
                  </TabsContent>
                </Tabs>
              </StepperContent>

              {/* 4 — verify the whole chain */}
              <StepperContent value={4} className="space-y-4">
                <p className="text-muted-foreground text-sm">
                  Kontrolliert den CNAME über öffentliche Resolver und testet, ob ein TXT-Wert
                  wirklich durchkommt — derselbe Weg, den eine Zertifizierungsstelle geht.
                </p>
                <Button onClick={() => onClose('check', record)}>
                  <Radar className="size-4" />
                  DNS-Einträge prüfen
                </Button>
              </StepperContent>
            </StepperPanel>
          </Stepper>

          <div className="flex flex-wrap items-end gap-3 border-t pt-4">
            <div className="min-w-56 flex-1 space-y-1.5">
              <Label htmlFor="target-domain" className="text-xs">
                Domain für die Vorlagen
              </Label>
              <Input
                id="target-domain"
                value={target}
                placeholder="beispiel.de"
                onChange={(e) => setTarget(e.target.value)}
              />
            </div>
            {nameDirty && (
              <Button variant="outline" disabled={busy} onClick={saveName}>
                <Save className="size-4" />
                Übernehmen
              </Button>
            )}
            {record.last_ip && (
              <p className="text-muted-foreground basis-full text-xs">
                Letzter Zugriff des Clients von{' '}
                <span className="font-mono">{record.last_ip}</span>
                {record.last_ip_host && ` (${record.last_ip_host})`}
              </p>
            )}
            <p className="text-muted-foreground basis-full text-xs">
              Ändert die oben erzeugten Blöcke. Angelegt am{' '}
              {record.created_at
                ? new Date(record.created_at * 1000).toLocaleDateString('de-DE')
                : '—'}
              {record.last_active
                ? `, zuletzt aktiv am ${new Date(record.last_active * 1000).toLocaleDateString('de-DE')}.`
                : ', noch nie genutzt.'}
            </p>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              disabled={currentStep === 1}
              onClick={() => setCurrentStep((s) => s - 1)}
            >
              Zurück
            </Button>
            {currentStep < STEPS.length ? (
              <Button onClick={() => setCurrentStep((s) => s + 1)}>Weiter</Button>
            ) : (
              <Button onClick={() => onClose('done', record)}>Fertig</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={rotateOpen}
        title="Neue Zugangsdaten erzeugen?"
        description="Das bisherige Passwort wird sofort ungültig. Der CNAME bleibt gültig — es muss nur die Client-Konfiguration (Traefik, certbot) mit dem neuen Passwort aktualisiert werden."
        confirmLabel="Neu erzeugen"
        destructive
        onConfirm={rotate}
        onOpenChange={setRotateOpen}
      />
    </>
  )
}
