import { Component, OnInit, ViewChild, ElementRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MatToolbarModule } from '@angular/material/toolbar';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatTableModule } from '@angular/material/table';
import { MatCardModule } from '@angular/material/card';
import { MatDialogModule, MatDialog } from '@angular/material/dialog';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { AuthService } from '../../services/auth.service';
import { AcmeDnsService } from '../../services/acme-dns.service';
import { AcmeDomain } from '../../models/domain.model';
import { RegisterDomainComponent } from '../register-domain/register-domain.component';
import { DnsCheckComponent } from '../dns-check/dns-check.component';
import { DomainDetailsComponent } from '../domain-details/domain-details.component';

@Component({
  selector: 'app-dashboard',
  imports: [
    CommonModule,
    FormsModule,
    MatToolbarModule,
    MatButtonModule,
    MatIconModule,
    MatTableModule,
    MatCardModule,
    MatDialogModule,
    MatTooltipModule,
    MatProgressSpinnerModule,
    MatSnackBarModule
  ],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss'
})
export class DashboardComponent implements OnInit {
  domains: AcmeDomain[] = [];
  displayedColumns: string[] = ['domain_name', 'fulldomain', 'last_active', 'actions'];
  loading = false;
  serverStatus: 'online' | 'offline' | 'checking' = 'checking';
  editingDomain: string | null = null;

  originalName: string = '';

  constructor(
    private authService: AuthService,
    private acmeDnsService: AcmeDnsService,
    private dialog: MatDialog,
    private snackBar: MatSnackBar
  ) {}

  ngOnInit(): void {
    this.loadDomains();
    this.checkServerHealth();
  }

  loadDomains(): void {
    this.loading = true;
    // Try to fetch from server first
    this.acmeDnsService.fetchDomainsFromServer().subscribe({
      next: (domains) => {
        this.domains = domains;
        this.loading = false;
      },
      error: (error) => {
        console.error('Error loading domains from server, using local storage:', error);
        // Fallback to localStorage
        this.acmeDnsService.getDomains().subscribe({
          next: (domains) => {
            this.domains = domains;
            this.loading = false;
          },
          error: (err) => {
            console.error('Error loading domains:', err);
            this.loading = false;
          }
        });
      }
    });
  }

  checkServerHealth(): void {
    this.serverStatus = 'checking';
    this.acmeDnsService.getHealth().subscribe({
      next: (response) => {
        this.serverStatus = response.status === 'online' ? 'online' : 'offline';
      },
      error: () => {
        this.serverStatus = 'offline';
      }
    });
  }

  openRegisterDialog(): void {
    const dialogRef = this.dialog.open(RegisterDomainComponent, {
      width: '500px'
    });

    dialogRef.afterClosed().subscribe(result => {
      if (result) {
        this.loadDomains();
      }
    });
  }

  copyToClipboard(text: string): void {
    navigator.clipboard.writeText(text);
  }

  deleteDomain(domain: AcmeDomain): void {
    const message = `Are you sure you want to remove ${domain.domain_name || domain.fulldomain} from this UI?

Note: This will only remove it from the local browser storage. The ACME-DNS registration on the server will remain active.`;
    
    if (confirm(message)) {
      this.acmeDnsService.deleteDomain(domain.fulldomain).subscribe(() => {
        this.snackBar.open('Domain removed from local storage', 'Close', {
          duration: 3000
        });
        this.loadDomains();
      });
    }
  }

  checkDNS(domain: AcmeDomain): void {
    const dialogRef = this.dialog.open(DnsCheckComponent, {
      width: '600px',
      data: { domain }
    });
  }

  showDetails(domain: AcmeDomain): void {
    const dialogRef = this.dialog.open(DomainDetailsComponent, {
      width: '600px',
      data: domain
    });
  }

  startEdit(domain: AcmeDomain): void {
    this.originalName = domain.domain_name || '';
    this.editingDomain = domain.fulldomain;
    // Focus input after Angular renders it
    setTimeout(() => {
      const input = document.querySelector('.name-input') as HTMLInputElement;
      if (input) {
        input.focus();
        input.select();
      }
    }, 100);
  }

  saveName(domain: AcmeDomain): void {
    if (this.editingDomain !== domain.fulldomain) return;
    
    // Update in service/storage
    this.acmeDnsService.updateDomainName(domain.fulldomain, domain.domain_name || '').subscribe({
      next: () => {
        this.snackBar.open('Name updated successfully', 'Close', {
          duration: 2000
        });
        this.editingDomain = null;
      },
      error: (error) => {
        console.error('Error updating domain name:', error);
        this.snackBar.open('Failed to update name', 'Close', {
          duration: 3000
        });
        // Revert to original name
        domain.domain_name = this.originalName;
        this.editingDomain = null;
      }
    });
  }

  cancelEdit(): void {
    // Find the domain and revert its name
    const domain = this.domains.find(d => d.fulldomain === this.editingDomain);
    if (domain) {
      domain.domain_name = this.originalName;
    }
    this.editingDomain = null;
  }

  logout(): void {
    this.authService.logout();
  }
}
