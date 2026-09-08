import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { AcmeDomain, DnsCheckResult, ServerInfo } from './models';

/** Thin wrapper around the /api/admin endpoints. */
@Injectable({ providedIn: 'root' })
export class ApiService {
  private http = inject(HttpClient);

  serverInfo(): Observable<ServerInfo> {
    return this.http.get<ServerInfo>('api/admin/server');
  }

  listDomains(): Observable<AcmeDomain[]> {
    return this.http.get<AcmeDomain[]>('api/admin/domains');
  }

  createDomain(domainName: string, allowFrom: string[] = []): Observable<AcmeDomain> {
    return this.http.post<AcmeDomain>('api/admin/domains', {
      domain_name: domainName,
      allowfrom: allowFrom,
    });
  }

  renameDomain(subdomain: string, domainName: string): Observable<AcmeDomain> {
    return this.http.post<AcmeDomain>(`api/admin/domains/${subdomain}/name`, {
      domain_name: domainName,
    });
  }

  /** Issues a new password. The subdomain and therefore the CNAME stay valid. */
  rotateCredentials(subdomain: string): Observable<AcmeDomain> {
    return this.http.post<AcmeDomain>(`api/admin/domains/${subdomain}/rotate`, {});
  }

  deleteDomain(subdomain: string): Observable<{ success: boolean }> {
    return this.http.delete<{ success: boolean }>(`api/admin/domains/${subdomain}`);
  }

  checkDns(domain: string, subdomain: string, skipTxt = false): Observable<DnsCheckResult> {
    return this.http.post<DnsCheckResult>('api/admin/dnscheck', {
      domain,
      subdomain,
      skip_txt: skipTxt,
    });
  }
}
