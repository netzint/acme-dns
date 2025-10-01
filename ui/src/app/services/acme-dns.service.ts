import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Observable, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';
import { AcmeDomain } from '../models/domain.model';
import { environment } from '../environments/environment';

@Injectable({
  providedIn: 'root'
})
export class AcmeDnsService {
  private apiUrl = environment.apiUrl;
  private domains: Map<string, AcmeDomain> = new Map();
  private apiKey = 'acme-dns-ui-key'; // You can make this configurable
  
  private getApiUrl(endpoint: string): string {
    // If apiUrl is empty, use same origin
    return this.apiUrl ? `${this.apiUrl}${endpoint}` : endpoint;
  }

  constructor(private http: HttpClient) {
    this.loadDomainsFromServer();
  }

  private loadDomainsFromServer(): void {
    // First try to load from server
    this.fetchDomainsFromServer().subscribe({
      next: (domains) => {
        // Server domains loaded successfully
        console.log('Loaded domains from server:', domains.length);
      },
      error: () => {
        // If server fails, load from localStorage
        this.loadDomainsFromStorage();
      }
    });
  }

  private loadDomainsFromStorage(): void {
    const stored = localStorage.getItem('acme_domains');
    if (stored) {
      const domainsArray = JSON.parse(stored) as AcmeDomain[];
      domainsArray.forEach(domain => {
        this.domains.set(domain.fulldomain, domain);
      });
    }
  }

  private saveDomainsToStorage(): void {
    const domainsArray = Array.from(this.domains.values());
    localStorage.setItem('acme_domains', JSON.stringify(domainsArray));
  }

  fetchDomainsFromServer(): Observable<AcmeDomain[]> {
    const headers = new HttpHeaders({
      'X-Api-Key': this.apiKey
    });

    return this.http.get<any[]>(this.getApiUrl('/domains'), { headers }).pipe(
      map(response => {
        // Clear existing domains
        this.domains.clear();
        
        // Map server response to our domain model
        const domains = response.map(item => {
          const domain: AcmeDomain = {
            subdomain: item.subdomain,
            username: item.username,
            password: '', // Password is not returned from server for security
            fulldomain: item.fulldomain,
            allowfrom: item.allowfrom,
            created_at: item.created_at,
            updated_at: item.updated_at
          };
          this.domains.set(domain.fulldomain, domain);
          return domain;
        });
        
        // Also merge with localStorage to preserve passwords
        const stored = localStorage.getItem('acme_domains');
        if (stored) {
          const storedDomains = JSON.parse(stored) as AcmeDomain[];
          storedDomains.forEach(storedDomain => {
            const existing = this.domains.get(storedDomain.fulldomain);
            if (existing && storedDomain.password) {
              // Preserve password from localStorage
              existing.password = storedDomain.password;
            }
          });
        }
        
        this.saveDomainsToStorage();
        return domains;
      }),
      catchError(error => {
        console.error('Error fetching domains from server:', error);
        // Return domains from localStorage as fallback
        return of(Array.from(this.domains.values()));
      })
    );
  }

  registerDomain(): Observable<AcmeDomain> {
    return this.http.post<AcmeDomain>(this.getApiUrl('/register'), {}).pipe(
      map(response => {
        const domain: AcmeDomain = {
          ...response,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        };
        this.domains.set(domain.fulldomain, domain);
        this.saveDomainsToStorage();
        return domain;
      }),
      catchError(error => {
        console.error('Error registering domain:', error);
        throw error;
      })
    );
  }

  updateDomain(domain: AcmeDomain, txt: string): Observable<any> {
    const headers = new HttpHeaders({
      'X-Api-User': domain.username,
      'X-Api-Key': domain.password
    });

    return this.http.post(this.getApiUrl('/update'), {
      subdomain: domain.subdomain,
      txt: txt
    }, { headers }).pipe(
      map(response => {
        domain.updated_at = new Date().toISOString();
        domain.last_active = new Date().toISOString();
        this.domains.set(domain.fulldomain, domain);
        this.saveDomainsToStorage();
        return response;
      }),
      catchError(error => {
        console.error('Error updating domain:', error);
        throw error;
      })
    );
  }

  getDomains(): Observable<AcmeDomain[]> {
    return of(Array.from(this.domains.values()));
  }

  getDomain(fulldomain: string): Observable<AcmeDomain | undefined> {
    return of(this.domains.get(fulldomain));
  }

  deleteDomain(fulldomain: string): Observable<boolean> {
    const deleted = this.domains.delete(fulldomain);
    if (deleted) {
      this.saveDomainsToStorage();
    }
    return of(deleted);
  }

  getHealth(): Observable<any> {
    return this.http.get(this.getApiUrl('/health')).pipe(
      catchError(error => {
        console.error('Health check failed:', error);
        return of({ status: 'error', message: 'Server unreachable' });
      })
    );
  }
}