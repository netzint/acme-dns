import { Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatMenuModule } from '@angular/material/menu';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSnackBar } from '@angular/material/snack-bar';
import { MatTooltipModule } from '@angular/material/tooltip';
import { ApiService } from '../../core/api.service';
import { AuthService } from '../../core/auth.service';
import { CopyService } from '../../core/copy.service';
import { AcmeDomain, ServerInfo } from '../../core/models';
import { ConfirmComponent } from '../../components/confirm/confirm.component';
import { CreateDomainComponent } from '../../components/create-domain/create-domain.component';
import {
  DnsCheckComponent,
  DnsCheckData,
} from '../../components/dns-check/dns-check.component';
import {
  DomainDetailsComponent,
  DomainDetailsData,
  DomainDetailsResult,
} from '../../components/domain-details/domain-details.component';

@Component({
  selector: 'app-dashboard',
  imports: [
    DatePipe,
    FormsModule,
    MatButtonModule,
    MatDialogModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatMenuModule,
    MatProgressBarModule,
    MatTooltipModule,
  ],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss',
})
export class DashboardComponent {
  private api = inject(ApiService);
  private dialog = inject(MatDialog);
  private snackBar = inject(MatSnackBar);
  protected auth = inject(AuthService);
  protected copy = inject(CopyService);

  readonly domains = signal<AcmeDomain[]>([]);
  readonly server = signal<ServerInfo | null>(null);
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly filter = signal('');

  readonly visibleDomains = computed(() => {
    const needle = this.filter().trim().toLowerCase();
    const all = this.domains();
    if (!needle) {
      return all;
    }
    return all.filter(
      (d) =>
        d.domain_name?.toLowerCase().includes(needle) ||
        d.fulldomain.toLowerCase().includes(needle) ||
        d.username.toLowerCase().includes(needle)
    );
  });

  constructor() {
    this.reload();
    this.api.serverInfo().subscribe({
      next: (info) => this.server.set(info),
      error: () => undefined,
    });
  }

  reload(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listDomains().subscribe({
      next: (domains) => {
        this.domains.set(
          [...domains].sort((a, b) => (b.created_at ?? 0) - (a.created_at ?? 0))
        );
        this.loading.set(false);
      },
      error: (response) => {
        this.loading.set(false);
        if (response.status !== 401) {
          this.loadError.set('Die Domainliste konnte nicht geladen werden.');
        }
      },
    });
  }

  create(): void {
    const ref = this.dialog.open(CreateDomainComponent, { autoFocus: 'first-tabbable' });
    ref.afterClosed().subscribe((created?: AcmeDomain) => {
      if (created) {
        this.reload();
        this.openDetails(created, true);
      }
    });
  }

  openDetails(domain: AcmeDomain, isNew = false): void {
    const server = this.server();
    if (!server) {
      this.snackBar.open('Server-Informationen sind noch nicht geladen', 'OK', { duration: 3000 });
      return;
    }

    const data: DomainDetailsData = { domain, server, isNew };
    const ref = this.dialog.open(DomainDetailsComponent, { data, maxHeight: '90vh' });

    ref.afterClosed().subscribe((result: DomainDetailsResult) => {
      if (!result) {
        return;
      }
      this.reload();
      if (result.action === 'check') {
        this.checkDns(result.domain);
      }
    });
  }

  checkDns(domain: AcmeDomain): void {
    const data: DnsCheckData = { domain };
    this.dialog.open(DnsCheckComponent, { data, maxHeight: '90vh' });
  }

  remove(domain: AcmeDomain): void {
    const label = domain.domain_name || domain.fulldomain;
    const ref = this.dialog.open(ConfirmComponent, {
      data: {
        title: 'Registrierung löschen?',
        message: `"${label}" wird endgültig vom Server entfernt.\n\nDer CNAME _acme-challenge.${label} zeigt danach ins Leere und Zertifikatsverlängerungen für diese Domain schlagen fehl.`,
        confirmLabel: 'Endgültig löschen',
        danger: true,
      },
    });

    ref.afterClosed().subscribe((confirmed) => {
      if (!confirmed) {
        return;
      }
      this.api.deleteDomain(domain.subdomain).subscribe({
        next: () => {
          this.snackBar.open('Registrierung gelöscht', undefined, { duration: 2500 });
          this.reload();
        },
        error: () => this.snackBar.open('Löschen fehlgeschlagen', 'OK', { duration: 4000 }),
      });
    });
  }

  logout(): void {
    this.auth.logout();
  }
}
