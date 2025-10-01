import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MatDialogRef, MatDialogModule } from '@angular/material/dialog';
import { MatButtonModule } from '@angular/material/button';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatIconModule } from '@angular/material/icon';
import { MatListModule } from '@angular/material/list';
import { MatInputModule } from '@angular/material/input';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { AcmeDnsService } from '../../services/acme-dns.service';
import { AcmeDomain } from '../../models/domain.model';

@Component({
  selector: 'app-register-domain',
  imports: [
    CommonModule,
    FormsModule,
    MatDialogModule,
    MatButtonModule,
    MatProgressSpinnerModule,
    MatIconModule,
    MatListModule,
    MatInputModule,
    MatFormFieldModule,
    MatSnackBarModule
  ],
  templateUrl: './register-domain.component.html',
  styleUrl: './register-domain.component.scss'
})
export class RegisterDomainComponent implements OnInit {
  loading = false;
  askingForName = true;
  domainName = '';
  newDomain: AcmeDomain | null = null;
  error: string | null = null;

  constructor(
    private dialogRef: MatDialogRef<RegisterDomainComponent>,
    private acmeDnsService: AcmeDnsService,
    private snackBar: MatSnackBar
  ) {}

  ngOnInit(): void {
    // Don't auto-register, wait for name input
  }

  submitName(): void {
    if (!this.domainName.trim()) {
      this.snackBar.open('Please enter a domain name', 'Close', {
        duration: 2000
      });
      return;
    }
    this.askingForName = false;
    this.registerDomain();
  }

  registerDomain(): void {
    this.loading = true;
    this.error = null;
    
    this.acmeDnsService.registerDomainWithName(this.domainName).subscribe({
      next: (domain) => {
        this.newDomain = domain;
        this.loading = false;
        this.snackBar.open('Domain registered successfully!', 'Close', {
          duration: 3000
        });
      },
      error: (error) => {
        this.error = 'Failed to register domain. Please check if the ACME-DNS server is running.';
        this.loading = false;
        console.error('Registration error:', error);
      }
    });
  }

  copyToClipboard(text: string, label: string): void {
    navigator.clipboard.writeText(text).then(() => {
      this.snackBar.open(`${label} copied to clipboard`, 'Close', {
        duration: 2000
      });
    });
  }

  copyAllAsJson(): void {
    if (this.newDomain) {
      const jsonData = JSON.stringify(this.newDomain, null, 2);
      navigator.clipboard.writeText(jsonData).then(() => {
        this.snackBar.open('All data copied as JSON', 'Close', {
          duration: 2000
        });
      });
    }
  }

  close(): void {
    this.dialogRef.close(this.newDomain);
  }
}
