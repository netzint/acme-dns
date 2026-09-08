import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MAT_DIALOG_DATA, MatDialogModule } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';
import { ApiService } from '../../core/api.service';
import { AcmeDomain, DnsCheckResult } from '../../core/models';
import { cnameFields, normalizeDomain } from '../../core/snippets';
import { CopyBlockComponent } from '../copy-block/copy-block.component';

export interface DnsCheckData {
  domain: AcmeDomain;
}

@Component({
  selector: 'app-dns-check',
  imports: [
    FormsModule,
    MatButtonModule,
    MatDialogModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressBarModule,
    MatSlideToggleModule,
    CopyBlockComponent,
  ],
  templateUrl: './dns-check.component.html',
  styleUrl: './dns-check.component.scss',
})
export class DnsCheckComponent {
  private api = inject(ApiService);
  readonly data = inject<DnsCheckData>(MAT_DIALOG_DATA);

  readonly domain = signal(this.data.domain.domain_name ?? '');
  readonly skipTxt = signal(false);
  readonly loading = signal(false);
  readonly result = signal<DnsCheckResult | null>(null);
  readonly error = signal<string | null>(null);

  readonly expected = computed(() => cnameFields(this.data.domain, this.domain() || 'beispiel.de'));

  readonly recordSnippet = computed(() => {
    const fields = this.expected();
    return `${fields.name}.\tIN\tCNAME\t${fields.target}`;
  });

  run(): void {
    const target = normalizeDomain(this.domain());
    if (!target || this.loading()) {
      return;
    }
    this.loading.set(true);
    this.error.set(null);
    this.result.set(null);

    this.api.checkDns(target, this.data.domain.subdomain, this.skipTxt()).subscribe({
      next: (result) => {
        this.loading.set(false);
        this.result.set(result);
      },
      error: (response) => {
        this.loading.set(false);
        this.error.set(
          response.status === 404
            ? 'Diese Registrierung existiert auf dem Server nicht mehr.'
            : 'Die Prüfung konnte nicht ausgeführt werden.'
        );
      },
    });
  }

  reset(): void {
    this.result.set(null);
    this.error.set(null);
  }

  iconFor(status: string): string {
    if (status === 'ok') {
      return 'check_circle';
    }
    return status === 'failed' ? 'cancel' : 'remove_circle_outline';
  }
}
