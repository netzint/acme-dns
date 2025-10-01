export interface AcmeDomain {
  subdomain: string;
  username: string;
  password: string;
  fulldomain: string;
  allowfrom?: string[];
  created_at?: string;
  updated_at?: string;
  last_active?: string;
}