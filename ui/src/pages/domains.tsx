import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  type ColumnDef,
  type PaginationState,
  type SortingState,
  useTable,
} from '@tanstack/react-table'
import {
  LogOut,
  MoreVertical,
  Plus,
  Radar,
  RefreshCw,
  Search,
  Server,
  Settings2,
  Trash2,
  Waypoints,
  TriangleAlert,
} from 'lucide-react'
import {
  DataGrid,
  DataGridContainer,
  dataGridFeatures,
  type DataGridFeatures,
} from '@/components/reui/data-grid/data-grid'
import { DataGridColumnHeader } from '@/components/reui/data-grid/data-grid-column-header'
import { DataGridPagination } from '@/components/reui/data-grid/data-grid-pagination'
import { DataGridScrollArea } from '@/components/reui/data-grid/data-grid-scroll-area'
import { DataGridTable } from '@/components/reui/data-grid/data-grid-table'
import { Alert, AlertDescription } from '@/components/reui/alert'
import { Badge } from '@/components/reui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { CreateDomainDialog } from '@/features/create-domain-dialog'
import { DnsCheckDialog } from '@/features/dns-check-dialog'
import { DomainDetailsDialog } from '@/features/domain-details-dialog'
import { ConfirmDialog } from '@/features/confirm-dialog'
import { MatchDomainsDialog } from '@/features/match-domains-dialog'
import { api, session } from '@/lib/api'
import { dataGridDe } from '@/lib/data-grid-de'
import type { AcmeDomain, ServerInfo } from '@/lib/types'

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

  const [createOpen, setCreateOpen] = useState(false)
  const [details, setDetails] = useState<{ domain: AcmeDomain; isNew: boolean } | null>(null)
  const [checking, setChecking] = useState<AcmeDomain | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AcmeDomain | null>(null)
  const [matchOpen, setMatchOpen] = useState(false)

  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 10 })
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
        d.username.toLowerCase().includes(needle),
    )
  }, [domains, filter])

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
    await reload()
  }

  const columns = useMemo<ColumnDef<DataGridFeatures, AcmeDomain>[]>(
    () => [
      {
        accessorKey: 'domain_name',
        header: ({ column }) => <DataGridColumnHeader column={column} title="Domain" />,
        size: 300,
        cell: ({ row }) => (
          <button
            type="button"
            className="group flex min-w-0 flex-col items-start py-1 text-left"
            onClick={() => setDetails({ domain: row.original, isNew: false })}
          >
            <span className="group-hover:text-primary font-medium transition-colors">
              {row.original.domain_name || '(ohne Namen)'}
            </span>
            <span className="text-muted-foreground max-w-full truncate font-mono text-[11px]">
              {row.original.fulldomain}
            </span>
          </button>
        ),
      },
      {
        accessorKey: 'last_active',
        header: ({ column }) => <DataGridColumnHeader column={column} title="Status" />,
        size: 190,
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
        size: 200,
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
              {host && <div className="truncate text-xs font-medium">{host}</div>}
              <div className="text-muted-foreground truncate font-mono text-[11px]">{ip}</div>
            </div>
          )
        },
      },
      {
        id: 'actions',
        header: '',
        size: 96,
        enableSorting: false,
        meta: { headerClassName: 'text-right', cellClassName: 'text-right' },
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-0.5">
            <Button
              size="icon"
              variant="ghost"
              className="size-8"
              title="DNS prüfen"
              onClick={() => setChecking(row.original)}
            >
              <Radar className="size-4" />
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button size="icon" variant="ghost" className="size-8" title="Weitere Aktionen">
                    <MoreVertical className="size-4" />
                  </Button>
                }
              />
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  onClick={() => setDetails({ domain: row.original, isNew: false })}
                >
                  <Settings2 className="size-4" />
                  Details und Vorlagen
                </DropdownMenuItem>
                <DropdownMenuItem variant="destructive" onClick={() => setPendingDelete(row.original)}>
                  <Trash2 className="size-4" />
                  Löschen
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
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
    state: { pagination, sorting },
    onPaginationChange: setPagination,
    onSortingChange: setSorting,
  })

  return (
    <div className="min-h-screen">
      <header className="bg-card/60 sticky top-0 z-10 border-b backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-4 px-5 py-3">
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
        </div>
      </header>

      <main className="mx-auto max-w-5xl space-y-5 px-5 py-7">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold tracking-tight">Domains</h1>
            <p className="text-muted-foreground text-sm">
              {domains.length} {domains.length === 1 ? 'Registrierung' : 'Registrierungen'}
            </p>
          </div>

          <div className="flex items-center gap-2">
            <div className="relative">
              <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
              <Input
                placeholder="Filtern"
                className="w-44 pl-8"
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value)
                  setPagination((p) => ({ ...p, pageIndex: 0 }))
                }}
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

        {loadError && (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertDescription>{loadError}</AlertDescription>
          </Alert>
        )}

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </div>
        ) : domains.length === 0 ? (
          <div className="rounded-xl border border-dashed py-16 text-center">
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
          <DataGrid table={table} recordCount={visible.length} i18n={dataGridDe}>
            <div className="w-full space-y-2.5">
              <DataGridContainer>
                <DataGridScrollArea>
                  <DataGridTable />
                </DataGridScrollArea>
              </DataGridContainer>
              {visible.length > pagination.pageSize && <DataGridPagination />}
            </div>
          </DataGrid>
        )}
      </main>

      <MatchDomainsDialog open={matchOpen} onOpenChange={setMatchOpen} onApplied={refresh} />

      <CreateDomainDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(domain) => {
          void reload()
          setDetails({ domain, isNew: true })
        }}
      />

      {details && server && (
        <DomainDetailsDialog
          domain={details.domain}
          server={server}
          isNew={details.isNew}
          onClose={(action, domain) => {
            setDetails(null)
            void reload()
            if (action === 'check') {
              setChecking(domain)
            }
          }}
        />
      )}

      {checking && (
        <DnsCheckDialog domain={checking} onClose={() => setChecking(null)} />
      )}

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
