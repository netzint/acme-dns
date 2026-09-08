import { Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap } from 'rxjs';
import { LoginResponse } from './models';

const TOKEN_KEY = 'acmedns.token';
const EXPIRY_KEY = 'acmedns.token.expires';
const USER_KEY = 'acmedns.user';

/**
 * Holds the management session. The token is issued by the server and only
 * lives as long as the server process, so a stored token can become invalid at
 * any time; the interceptor turns a 401 into a logout.
 */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);
  private router = inject(Router);

  private tokenSignal = signal<string | null>(this.readValidToken());
  readonly username = signal<string>(localStorage.getItem(USER_KEY) ?? '');
  readonly isAuthenticated = computed(() => this.tokenSignal() !== null);

  private readValidToken(): string | null {
    const token = localStorage.getItem(TOKEN_KEY);
    const expires = Number(localStorage.getItem(EXPIRY_KEY) ?? 0);
    if (!token || !expires || expires * 1000 <= Date.now()) {
      this.clearStorage();
      return null;
    }
    return token;
  }

  private clearStorage(): void {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(EXPIRY_KEY);
    localStorage.removeItem(USER_KEY);
  }

  get token(): string | null {
    return this.tokenSignal();
  }

  login(username: string, password: string): Observable<LoginResponse> {
    return this.http.post<LoginResponse>('api/admin/login', { username, password }).pipe(
      tap((response) => {
        localStorage.setItem(TOKEN_KEY, response.token);
        localStorage.setItem(EXPIRY_KEY, String(response.expires_at));
        localStorage.setItem(USER_KEY, response.username);
        this.tokenSignal.set(response.token);
        this.username.set(response.username);
      })
    );
  }

  /** Drops the local session and returns to the login screen. */
  logout(navigate = true): void {
    const token = this.tokenSignal();
    this.clearStorage();
    this.tokenSignal.set(null);
    this.username.set('');
    if (token) {
      // Best effort: the session is gone locally either way.
      this.http.post('api/admin/logout', {}, { headers: { Authorization: `Bearer ${token}` } }).subscribe({
        error: () => undefined,
      });
    }
    if (navigate) {
      void this.router.navigate(['/login']);
    }
  }
}
