import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MatToolbarModule } from '@angular/material/toolbar';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatTableModule } from '@angular/material/table';
import { MatCardModule } from '@angular/material/card';
import { MatDialogModule, MatDialog } from '@angular/material/dialog';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { AuthService } from '../../services/auth.service';
import { AcmeDnsService } from '../../services/acme-dns.service';
import { AcmeDomain } from '../../models/domain.model';
import { RegisterDomainComponent } from '../register-domain/register-domain.component';

@Component({
  selector: 'app-dashboard',
  imports: [
    CommonModule,
    MatToolbarModule,
    MatButtonModule,
    MatIconModule,
    MatTableModule,
    MatCardModule,
    MatDialogModule,
    MatTooltipModule,
    MatProgressSpinnerModule
  ],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss'
})
export class DashboardComponent implements OnInit {
  domains: AcmeDomain[] = [];
  displayedColumns: string[] = ['domain_name', 'fulldomain', 'subdomain', 'username', 'created_at', 'last_active', 'actions'];
  loading = false;
  serverStatus: 'online' | 'offline' | 'checking' = 'checking';

  constructor(
    private authService: AuthService,
    private acmeDnsService: AcmeDnsService,
    private dialog: MatDialog
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
        this.serverStatus = response.status === 'error' ? 'offline' : 'online';
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
    if (confirm(`Are you sure you want to delete ${domain.fulldomain}?`)) {
      this.acmeDnsService.deleteDomain(domain.fulldomain).subscribe(() => {
        this.loadDomains();
      });
    }
  }

  logout(): void {
    this.authService.logout();
  }
}
