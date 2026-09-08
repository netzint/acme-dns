import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError } from 'rxjs';
import { AuthService } from './auth.service';

/**
 * Attaches the management session token to every API call and logs the user out
 * when the server rejects it, which happens after a server restart.
 */
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  const token = auth.token;

  const request =
    token && req.url.startsWith('api/')
      ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } })
      : req;

  return next(request).pipe(
    catchError((error: HttpErrorResponse) => {
      const isLogin = req.url.endsWith('api/admin/login');
      if (error.status === 401 && !isLogin) {
        auth.logout();
      }
      return throwError(() => error);
    })
  );
};
