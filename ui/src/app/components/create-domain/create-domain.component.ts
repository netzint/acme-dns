import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { ApiService } from '../../core/api.service';
import { AcmeDomain } from '../../core/models';
import { normalizeDomain } from '../../core/snippets';

/** Matches a hostname such as example.com or sub.example.co.uk. */
const DOMAIN_PATTERN = /^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

@Component({
  selector: 'app-create-domain',
  imports: [
    ReactiveFormsModule,
    MatButtonModule,
    MatDialogModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressBarModule,
  ],
  templateUrl: './create-domain.component.html',
  styleUrl: './create-domain.component.scss',
})
export class CreateDomainComponent {
  private fb = inject(FormBuilder);
  private api = inject(ApiService);
  readonly dialogRef = inject(MatDialogRef<CreateDomainComponent, AcmeDomain>);

  readonly loading = signal(false);
  readonly error = signal<string | null>(null);

  readonly form = this.fb.nonNullable.group({
    domain: ['', [Validators.required, Validators.pattern(DOMAIN_PATTERN)]],
  });

  submit(): void {
    if (this.form.invalid || this.loading()) {
      return;
    }
    this.loading.set(true);
    this.error.set(null);

    const domain = normalizeDomain(this.form.getRawValue().domain);
    this.api.createDomain(domain).subscribe({
      next: (created) => {
        this.loading.set(false);
        this.dialogRef.close(created);
      },
      error: () => {
        this.loading.set(false);
        this.error.set('Die Registrierung ist fehlgeschlagen. Bitte das Server-Log prüfen.');
      },
    });
  }
}
