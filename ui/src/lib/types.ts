/** A single acme-dns registration as returned by the management API. */
export interface AcmeDomain {
  username: string;
  /** Recoverable plaintext password. Empty when credentials_available is false. */
  password: string;
  /** False for records created before credential storage was enabled. */
  credentials_available: boolean;
  fulldomain: string;
  subdomain: string;
  allowfrom: string[];
  /** The customer domain this registration was created for, e.g. example.com */
  domain_name: string;
  created_at: number;
  updated_at: number;
  /** Unix seconds of the last TXT update, 0 when never used. */
  last_active: number;
}

/** Deployment specific values the UI needs to render client snippets. */
export interface ServerInfo {
  acme_dns_domain: string;
  api_base_url: string;
  credential_storage: boolean;
  registration_open: boolean;
}

export type DnsCheckStatus = 'ok' | 'failed' | 'skipped';

export interface DnsCheckStep {
  id: string;
  status: DnsCheckStatus;
  title: string;
  detail: string;
}

export interface DnsCheckResult {
  valid: boolean;
  has_cname: boolean;
  cname_target: string;
  expected: string;
  challenge: string;
  resolvers: string[];
  steps: DnsCheckStep[];
  message: string;
  error?: string;
}

export interface LoginResponse {
  token: string;
  expires_at: number;
  username: string;
}
