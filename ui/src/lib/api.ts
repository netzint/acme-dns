import type {
  AcmeDomain,
  DnsCheckResult,
  LoginResponse,
  MatchResponse,
  ServerInfo,
} from './types'

const TOKEN_KEY = 'acmedns.token'
const EXPIRY_KEY = 'acmedns.token.expires'
const USER_KEY = 'acmedns.user'

/** Thrown for any non-2xx response so callers can branch on the status. */
export class ApiError extends Error {
  readonly status: number
  readonly code?: string

  constructor(status: number, code?: string) {
    super(code ?? `HTTP ${status}`)
    this.status = status
    this.code = code
  }
}

export const session = {
  get token(): string | null {
    const token = localStorage.getItem(TOKEN_KEY)
    const expires = Number(localStorage.getItem(EXPIRY_KEY) ?? 0)
    if (!token || !expires || expires * 1000 <= Date.now()) {
      session.clear()
      return null
    }
    return token
  },
  get username(): string {
    return localStorage.getItem(USER_KEY) ?? ''
  },
  store(response: LoginResponse) {
    localStorage.setItem(TOKEN_KEY, response.token)
    localStorage.setItem(EXPIRY_KEY, String(response.expires_at))
    localStorage.setItem(USER_KEY, response.username)
  },
  clear() {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(EXPIRY_KEY)
    localStorage.removeItem(USER_KEY)
  },
}

/** Notified when the server rejects our token, so the app can send us to /login. */
let onUnauthorized: () => void = () => {}
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = session.token
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(path, { ...init, headers })

  if (!response.ok) {
    let code: string | undefined
    try {
      code = (await response.json())?.error
    } catch {
      // A non-JSON body carries no error code; the status is enough.
    }
    if (response.status === 401 && !path.endsWith('/login')) {
      session.clear()
      onUnauthorized()
    }
    throw new ApiError(response.status, code)
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export const api = {
  login: (username: string, password: string) =>
    request<LoginResponse>('/api/admin/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request<{ success: boolean }>('/api/admin/logout', { method: 'POST' }),

  serverInfo: () => request<ServerInfo>('/api/admin/server'),

  listDomains: () => request<AcmeDomain[]>('/api/admin/domains'),

  createDomain: (domainName: string) =>
    request<AcmeDomain>('/api/admin/domains', {
      method: 'POST',
      body: JSON.stringify({ domain_name: domainName, allowfrom: [] }),
    }),

  renameDomain: (subdomain: string, domainName: string) =>
    request<AcmeDomain>(`/api/admin/domains/${subdomain}/name`, {
      method: 'POST',
      body: JSON.stringify({ domain_name: domainName }),
    }),

  /** Issues a new password. The subdomain and therefore the CNAME stay valid. */
  rotateCredentials: (subdomain: string) =>
    request<AcmeDomain>(`/api/admin/domains/${subdomain}/rotate`, {
      method: 'POST',
      body: '{}',
    }),

  deleteDomain: (subdomain: string) =>
    request<{ success: boolean }>(`/api/admin/domains/${subdomain}`, { method: 'DELETE' }),

  /** Resolves _acme-challenge for each candidate and reports which registration it belongs to. */
  matchDomains: (domains: string[], apply: boolean) =>
    request<MatchResponse>('/api/admin/match', {
      method: 'POST',
      body: JSON.stringify({ domains, apply }),
    }),

  checkDns: (domain: string, subdomain: string, skipTxt: boolean) =>
    request<DnsCheckResult>('/api/admin/dnscheck', {
      method: 'POST',
      body: JSON.stringify({ domain, subdomain, skip_txt: skipTxt }),
    }),
}
