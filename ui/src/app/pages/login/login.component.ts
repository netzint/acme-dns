import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { AuthService } from '../../core/auth.service';

@Component({
  selector: 'app-login',
  imports: [
    ReactiveFormsModule,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressBarModule,
  ],
  templateUrl: './login.component.html',
  styleUrl: './login.component.scss',
})
export class LoginComponent {
  private fb = inject(FormBuilder);
  private auth = inject(AuthService);
  private router = inject(Router);

  readonly loading = signal(false);
  readonly error = signal<string | null>(null);

  readonly form = this.fb.nonNullable.group({
    username: ['', Validators.required],
    password: ['', Validators.required],
  });

  submit(): void {
    if (this.form.invalid || this.loading()) {
      return;
    }
    this.loading.set(true);
    this.error.set(null);

    const { username, password } = this.form.getRawValue();
    this.auth.login(username, password).subscribe({
      next: () => {
        this.loading.set(false);
        void this.router.navigate(['/domains']);
      },
      error: (response) => {
        this.loading.set(false);
        this.error.set(this.messageFor(response.status, response.error?.error));
      },
    });
  }

  private messageFor(status: number, code?: string): string {
    if (status === 429 || code === 'too_many_attempts') {
      return 'Zu viele Fehlversuche. Bitte in 15 Minuten erneut versuchen.';
    }
    if (status === 401) {
      return 'Benutzername oder Passwort ist falsch.';
    }
    if (status === 404) {
      return 'Die Verwaltung ist auf diesem Server nicht aktiviert (auth.admin_user fehlt in der config.cfg).';
    }
    if (status === 0) {
      return 'Der Server ist nicht erreichbar.';
    }
    return 'Anmeldung fehlgeschlagen.';
  }
}
