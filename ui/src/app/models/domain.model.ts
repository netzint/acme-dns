export interface AcmeDomain {
  subdomain: string;
  username: string;
  password: string;
  fulldomain: string;
  allowfrom?: string[];
  domain_name?: string;
  created_at?: number | string;
  updated_at?: number | string;
  last_active?: string;
}