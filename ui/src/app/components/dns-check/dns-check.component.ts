import { Component, Inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MatDialogRef, MatDialogModule, MAT_DIALOG_DATA } from '@angular/material/dialog';
import { MatButtonModule } from '@angular/material/button';
import { MatInputModule } from '@angular/material/input';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatListModule } from '@angular/material/list';
import { MatChipsModule } from '@angular/material/chips';
import { AcmeDnsService } from '../../services/acme-dns.service';
import { AcmeDomain } from '../../models/domain.model';

interface DNSCheckResult {
  valid: boolean;
  has_cname: boolean;
  cname_target: string;
  expected: string;
  error?: string;
  message: string;
  records?: string[];
}

@Component({
  selector: 'app-dns-check',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    MatDialogModule,
    MatButtonModule,
    MatInputModule,
    MatFormFieldModule,
    MatIconModule,
    MatProgressSpinnerModule,
    MatListModule,
    MatChipsModule
  ],
  templateUrl: './dns-check.component.html',
  styleUrl: './dns-check.component.scss'
})
export class DnsCheckComponent {
  domain: string = '';
  loading = false;
  result: DNSCheckResult | null = null;
  error: string | null = null;

  constructor(
    private dialogRef: MatDialogRef<DnsCheckComponent>,
    private acmeDnsService: AcmeDnsService,
    @Inject(MAT_DIALOG_DATA) public data: { domain: AcmeDomain }
  ) {
    // Pre-fill with domain name if available
    if (data?.domain?.domain_name) {
      // Try to extract just the domain from the name
      const name = data.domain.domain_name;
      // Remove common prefixes/protocols if present
      this.domain = name.replace(/^(https?:\/\/)?(www\.)?/i, '');
    }
  }

  checkDNS(): void {
    if (!this.domain.trim()) {
      this.error = 'Please enter a domain name';
      return;
    }

    if (!this.data?.domain?.fulldomain) {
      this.error = 'No ACME-DNS domain available';
      return;
    }

    this.loading = true;
    this.error = null;
    this.result = null;

    this.acmeDnsService.checkDNS(
      this.domain.trim(), 
      this.data.domain.subdomain,
      this.data.domain.fulldomain
    ).subscribe({
      next: (result) => {
        this.loading = false;
        this.result = result;
      },
      error: (error) => {
        this.loading = false;
        this.error = 'Failed to check DNS. Please try again.';
        console.error('DNS check error:', error);
      }
    });
  }

  getChallengeRecord(): string {
    return `_acme-challenge.${this.domain}`;
  }

  copyToClipboard(text: string): void {
    navigator.clipboard.writeText(text);
  }

  close(): void {
    this.dialogRef.close();
  }
}