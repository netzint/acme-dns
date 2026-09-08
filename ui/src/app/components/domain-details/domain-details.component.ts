import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { MatButtonModule } from '@angular/material/button';
import { MAT_DIALOG_DATA, MatDialog, MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatTabsModule } from '@angular/material/tabs';
import { MatSnackBar } from '@angular/material/snack-bar';
import { MatTooltipModule } from '@angular/material/tooltip';
import { ApiService } from '../../core/api.service';
import { CopyService } from '../../core/copy.service';
import { AcmeDomain, ServerInfo } from '../../core/models';
import {
  certbotSnippet,
  cnameFields,
  curlSnippet,
  legoStorageJson,
  normalizeDomain,
  traefikCompose,
  traefikStatic,
} from '../../core/snippets';
import { ConfirmComponent } from '../confirm/confirm.component';
import { CopyBlockComponent } from '../copy-block/copy-block.component';

export interface DomainDetailsData {
  domain: AcmeDomain;
  server: ServerInfo;
  /** True right after registration, which changes the wording to onboarding. */
  isNew?: boolean;
}

/** What the dialog asks the dashboard to do next, plus the possibly renamed record. */
export type DomainDetailsResult = { action: 'check' | 'refresh'; domain: AcmeDomain } | undefined;

@Component({
  selector: 'app-domain-details',
  imports: [
    FormsModule,
    DatePipe,
    MatButtonModule,
    MatDialogModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressBarModule,
    MatTabsModule,
    MatTooltipModule,
    CopyBlockComponent,
  ],
  templateUrl: './domain-details.component.html',
  styleUrl: './domain-details.component.scss',
})
export class DomainDetailsComponent {
  private api = inject(ApiService);
  private dialog = inject(MatDialog);
  private snackBar = inject(MatSnackBar);
  protected copy = inject(CopyService);
  readonly dialogRef = inject(MatDialogRef<DomainDetailsComponent, DomainDetailsResult>);
  readonly data = inject<DomainDetailsData>(MAT_DIALOG_DATA);

  readonly record = signal<AcmeDomain>(this.data.domain);
  readonly server = this.data.server;

  /** The certificate domain the snippets are generated for. */
  readonly target = signal(this.data.domain.domain_name ?? '');
  readonly showPassword = signal(this.data.isNew ?? false);
  readonly busy = signal(false);
  readonly nameDirty = computed(
    () => normalizeDomain(this.target()) !== normalizeDomain(this.record().domain_name ?? '')
  );

  readonly cname = computed(() => cnameFields(this.record(), this.target() || 'beispiel.de'));
  readonly cnameSnippet = computed(() => {
    const fields = this.cname();
    return `${fields.name}.\tIN\tCNAME\t${fields.target}`;
  });

  readonly legoJson = computed(() =>
    legoStorageJson(this.record(), this.target() || 'beispiel.de', this.server)
  );
  readonly traefikYaml = computed(() =>
    traefikCompose(this.record(), this.target() || 'beispiel.de', this.server)
  );
  readonly traefikStaticYaml = computed(() =>
    traefikStatic(this.record(), this.target() || 'beispiel.de', this.server)
  );
  readonly certbot = computed(() =>
    certbotSnippet(this.record(), this.target() || 'beispiel.de', this.server)
  );
  readonly curl = computed(() => curlSnippet(this.record(), this.server));

  readonly maskedPassword = computed(() => {
    const value = this.record().password;
    return value ? '•'.repeat(Math.min(value.length, 40)) : '';
  });

  saveName(): void {
    if (!this.nameDirty() || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.api.renameDomain(this.record().subdomain, normalizeDomain(this.target())).subscribe({
      next: (updated) => {
        // The server does not return the password on a rename, keep the local one.
        this.record.set({ ...updated, password: this.record().password });
        this.busy.set(false);
        this.snackBar.open('Domain gespeichert', undefined, { duration: 1800 });
      },
      error: () => {
        this.busy.set(false);
        this.snackBar.open('Speichern fehlgeschlagen', 'OK', { duration: 4000 });
      },
    });
  }

  rotate(): void {
    if (this.busy()) {
      return;
    }
    const ref = this.dialog.open(ConfirmComponent, {
      data: {
        title: 'Neue Zugangsdaten erzeugen?',
        message:
          'Das bisherige Passwort wird sofort ungültig. Der CNAME bleibt gültig — es muss nur die Client-Konfiguration (Traefik, certbot) mit dem neuen Passwort aktualisiert werden.',
        confirmLabel: 'Neu erzeugen',
        danger: true,
      },
    });

    ref.afterClosed().subscribe((confirmed) => {
      if (!confirmed) {
        return;
      }
      this.busy.set(true);
      this.api.rotateCredentials(this.record().subdomain).subscribe({
        next: (updated) => {
          this.record.set(updated);
          this.showPassword.set(true);
          this.busy.set(false);
          this.snackBar.open('Neue Zugangsdaten erzeugt', undefined, { duration: 2500 });
        },
        error: () => {
          this.busy.set(false);
          this.snackBar.open('Erzeugen fehlgeschlagen', 'OK', { duration: 4000 });
        },
      });
    });
  }

  openCheck(): void {
    this.dialogRef.close({ action: 'check', domain: this.record() });
  }

  close(): void {
    this.dialogRef.close({ action: 'refresh', domain: this.record() });
  }
}
