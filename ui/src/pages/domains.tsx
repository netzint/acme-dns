import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  type ColumnDef,
  type SortingState,
  useTable,
} from '@tanstack/react-table'
import {
  LogOut,
  MousePointerClick,
  Plus,
  Radar,
  RefreshCw,
  Search,
  Server,
  TriangleAlert,
  Waypoints,
} from 'lucide-react'
import {
  DataGrid,
  DataGridContainer,
  dataGridFeatures,
  type DataGridFeatures,
} from '@/components/reui/data-grid/data-grid'
import { DataGridColumnHeader } from '@/components/reui/data-grid/data-grid-column-header'
import { DataGridTable } from '@/components/reui/data-grid/data-grid-table'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Badge } from '@/components/reui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { CreateDomainDialog } from '@/features/create-domain-dialog'
import { DnsCheckDialog } from '@/features/dns-check-dialog'
import { DomainDetail } from '@/features/domain-detail'
import { ConfirmDialog } from '@/features/confirm-dialog'
import { MatchDomainsDialog } from '@/features/match-domains-dialog'
import { api, session } from '@/lib/api'
import { dataGridDe } from '@/lib/data-grid-de'
import type { AcmeDomain, ServerInfo } from '@/lib/types'
import { cn } from '@/lib/utils'

function formatDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleDateString('de-DE', {
    day: '2-digit',
    month: '2-digit',
    year: '2-digit',
  })
}

export default function DomainsPage() {
  const navigate = useNavigate()

  const [domains, setDomains] = useState<AcmeDomain[]>([])
  const [server, setServer] = useState<ServerInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [filter, setFilter] = useState('')

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [newlyCreatedId, setNewlyCreatedId] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [matchOpen, setMatchOpen] = useState(false)
  const [checking, setChecking] = useState<AcmeDomain | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AcmeDomain | null>(null)

  const [sorting, setSorting] = useState<SortingState>([{ id: 'last_active', desc: true }])

  // No synchronous state update here: the effect below calls this on mount, and
  // the spinner is already the initial state. Callers that refetch on a user
  // action flip `loading` themselves.
  const reload = useCallback(async () => {
    try {
      setDomains(await api.listDomains())
      setLoadError(null)
    } catch {
      setLoadError('Die Domainliste konnte nicht geladen werden.')
    } finally {
      setLoading(false)
    }
  }, [])

  const refresh = useCallback(() => {
    setLoading(true)
    void reload()
  }, [reload])

  useEffect(() => {
    // The rule cannot see that reload only touches state after its first await,
    // so nothing here renders synchronously.
    // oxlint-disable-next-line react/set-state-in-effect
    void reload()
    api
      .serverInfo()
      .then(setServer)
      .catch(() => undefined)
  }, [reload])

  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    if (!needle) {
      return domains
    }
    return domains.filter(
      (d) =>
        d.domain_name?.toLowerCase().includes(needle) ||
        d.fulldomain.toLowerCase().includes(needle) ||
        d.username.toLowerCase().includes(needle) ||
        d.last_ip_host?.toLowerCase().includes(needle),
    )
  }, [domains, filter])

  // Resolve the selection against the freshly loaded list so the pane always
  // shows current data, and drop it when the row disappears.
  const selected = useMemo(
    () => domains.find((d) => d.subdomain === selectedId) ?? null,
    [domains, selectedId],
  )

  async function logout() {
    try {
      await api.logout()
    } catch {
      // The session is gone locally either way.
    }
    session.clear()
    navigate('/login', { replace: true })
  }

  async function confirmDelete() {
    const domain = pendingDelete
    if (!domain) {
      return
    }
    setPendingDelete(null)
    await api.deleteDomain(domain.subdomain)
    if (selectedId === domain.subdomain) {
      setSelectedId(null)
    }
    await reload()
  }

  const columns = useMemo<ColumnDef<DataGridFeatures, AcmeDomain>[]>(
    () => [
      {
        accessorKey: 'domain_name',
        header: ({ column }) => <DataGridColumnHeader column={column} title="Domain" />,
        size: 340,
        meta: { cellClassName: 'py-3.5' },
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">
              {row.original.domain_name || 'Ohne Namen'}
            </div>
            <div className="text-muted-foreground truncate font-mono text-xs">
              {row.original.subdomain}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'last_active',
        header: ({ column }) => <DataGridColumnHeader column={column} title="Status" />,
        size: 200,
        meta: { cellClassName: 'py-3.5' },
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1.5">
            {row.original.last_active ? (
              <Badge variant="success-light" title="Zuletzt wurde ein TXT-Wert geschrieben">
                aktiv · {formatDate(row.original.last_active)}
              </Badge>
            ) : (
              <Badge variant="outline" title="Es wurde noch nie ein TXT-Wert gesetzt">
                ungenutzt
              </Badge>
            )}
            {!row.original.credentials_available && (
              <Badge variant="warning-light" title="Passwort nicht wiederherstellbar">
                kein Passwort
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'last_ip',
        header: ({ column }) => <DataGridColumnHeader column={column} title="Herkunft" />,
        size: 220,
        meta: { cellClassName: 'py-3.5' },
        cell: ({ row }) => {
          const { last_ip: ip, last_ip_host: host } = row.original
          if (!ip) {
            return (
              <span
                className="text-muted-foreground text-xs"
                title="Wird beim nächsten /update des Clients aufgezeichnet"
              >
                noch unbekannt
              </span>
            )
          }
          return (
            <div className="min-w-0">
              {host && <div className="truncate text-sm">{host}</div>}
              <div className="text-muted-foreground truncate font-mono text-xs">{ip}</div>
            </div>
          )
        },
      },
      {
        id: 'actions',
        header: '',
        size: 60,
        enableSorting: false,
        meta: { headerClassName: 'text-right', cellClassName: 'text-right py-3.5' },
        cell: ({ row }) => (
          <Button
            size="icon"
            variant="ghost"
            className="size-8"
            title="DNS prüfen"
            onClick={(e) => {
              e.stopPropagation()
              setChecking(row.original)
            }}
          >
            <Radar className="size-4" />
          </Button>
        ),
      },
    ],
    [],
  )

  const table = useTable({
    features: dataGridFeatures,
    columns,
    data: visible,
    getRowId: (row: AcmeDomain) => row.subdomain,
    enableRowSelection: true,
    enableMultiRowSelection: false,
    state: {
      sorting,
      // Mirrors the detail pane's selection so the row highlights.
      rowSelection: selectedId ? { [selectedId]: true } : {},
    },
    onSortingChange: setSorting,
  })

  return (
    <div className="flex min-h-screen flex-col xl:h-screen xl:overflow-hidden">
      <header className="flex shrink-0 items-center justify-between gap-4 border-b px-6 py-3">
        <div className="flex items-center gap-3">
          <span className="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg">
            <Server className="size-4.5" />
          </span>
          <div>
            <p className="text-sm leading-tight font-semibold">acme-dns</p>
            {server && (
              <p className="text-muted-foreground font-mono text-[11px] leading-tight">
                {server.acme_dns_domain}
              </p>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {server && !server.credential_storage && (
            <Badge
              variant="warning-light"
              title="auth.credentials_key ist nicht gesetzt — neue Passwörter lassen sich später nicht mehr anzeigen"
            >
              Passwortspeicherung aus
            </Badge>
          )}
          <span className="text-muted-foreground hidden text-xs sm:inline">
            {session.username}
          </span>
          <Button size="icon" variant="ghost" className="size-8" title="Abmelden" onClick={logout}>
            <LogOut className="size-4" />
          </Button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1 flex-col xl:flex-row">
        {/* Master: the list fills whatever the detail pane leaves. */}
        <section className="flex min-w-0 flex-1 flex-col">
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 px-6 py-4">
            <div className="flex items-baseline gap-2.5">
              <h1 className="text-lg font-semibold tracking-tight">Domains</h1>
              <span className="text-muted-foreground text-sm">
                {domains.length} {domains.length === 1 ? 'Registrierung' : 'Registrierungen'}
              </span>
            </div>

            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
                <Input
                  placeholder="Filtern"
                  className="w-52 pl-8"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                />
              </div>
              <Button size="icon" variant="outline" title="Neu laden" onClick={refresh}>
                <RefreshCw className="size-4" />
              </Button>
              <Button
                variant="outline"
                title="Unbenannte Registrierungen über ihren CNAME zuordnen"
                onClick={() => setMatchOpen(true)}
              >
                <Waypoints className="size-4" />
                Zuordnen
              </Button>
              <Button onClick={() => setCreateOpen(true)}>
                <Plus className="size-4" />
                Neue Domain
              </Button>
            </div>
          </div>

          <div className="min-h-0 flex-1 px-6 pb-6 xl:overflow-auto">
            {loadError && (
              <Alert variant="destructive" className="mb-4">
                <TriangleAlert />
                <AlertDescription>{loadError}</AlertDescription>
              </Alert>
            )}

            {loading ? (
              <div className="space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-16 w-full" />
                ))}
              </div>
            ) : domains.length === 0 ? (
              <div className="rounded-xl border border-dashed py-20 text-center">
                <span className="bg-primary/10 text-primary mx-auto mb-4 flex size-11 items-center justify-center rounded-xl">
                  <Server className="size-5" />
                </span>
                <h2 className="font-semibold">Noch keine Domain angelegt</h2>
                <p className="text-muted-foreground mx-auto mt-1.5 max-w-md text-sm">
                  Eine Domain anlegen, den erzeugten CNAME einmalig in der Zone eintragen,
                  Zugangsdaten in Traefik oder certbot hinterlegen — danach laufen alle Zertifikate
                  automatisch.
                </p>
                <Button className="mt-5" onClick={() => setCreateOpen(true)}>
                  <Plus className="size-4" />
                  Erste Domain anlegen
                </Button>
              </div>
            ) : (
              <DataGrid
                table={table}
                recordCount={visible.length}
                i18n={dataGridDe}
                tableLayout={{ headerSticky: true, rowBorder: true }}
                onRowClick={(row: AcmeDomain) => {
                  setSelectedId(row.subdomain)
                  setNewlyCreatedId(null)
                }}
                tableClassNames={{
                  bodyRow: 'data-[state=selected]:bg-primary/10',
                }}
              >
                <DataGridContainer border={false}>
                  <DataGridTable />
                </DataGridContainer>
              </DataGrid>
            )}
          </div>
        </section>

        {/* Detail: always present on a wide screen, so nothing hides in a modal. */}
        <aside
          className={cn(
            'shrink-0 border-t xl:w-[46%] xl:max-w-[860px] xl:min-w-[520px] xl:border-t-0 xl:border-l',
            selected ? 'block' : 'hidden xl:block',
          )}
        >
          {selected && server ? (
            <DomainDetail
              key={selected.subdomain}
              domain={selected}
              server={server}
              isNew={newlyCreatedId === selected.subdomain}
              onCheck={setChecking}
              onDelete={setPendingDelete}
              onChanged={reload}
            />
          ) : (
            <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-3 px-10 text-center">
              <MousePointerClick className="size-7 opacity-40" />
              <p className="text-sm">
                Eine Zeile auswählen, um CNAME, Zugangsdaten und die fertigen
                Client-Konfigurationen zu sehen.
              </p>
            </div>
          )}
        </aside>
      </div>

      <MatchDomainsDialog open={matchOpen} onOpenChange={setMatchOpen} onApplied={refresh} />

      <CreateDomainDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(domain) => {
          void reload()
          setSelectedId(domain.subdomain)
          setNewlyCreatedId(domain.subdomain)
        }}
      />

      {checking && <DnsCheckDialog domain={checking} onClose={() => setChecking(null)} />}

      <ConfirmDialog
        open={pendingDelete !== null}
        title="Registrierung löschen?"
        description={`"${pendingDelete?.domain_name || pendingDelete?.fulldomain}" wird endgültig vom Server entfernt.\n\nDer CNAME zeigt danach ins Leere und Zertifikatsverlängerungen für diese Domain schlagen fehl.`}
        confirmLabel="Endgültig löschen"
        destructive
        onConfirm={confirmDelete}
        onOpenChange={(open) => !open && setPendingDelete(null)}
      />
    </div>
  )
}
