import { Component, Inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MatDialogRef, MatDialogModule, MAT_DIALOG_DATA } from '@angular/material/dialog';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatListModule } from '@angular/material/list';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { AcmeDomain } from '../../models/domain.model';

@Component({
  selector: 'app-domain-details',
  standalone: true,
  imports: [
    CommonModule,
    MatDialogModule,
    MatButtonModule,
    MatIconModule,
    MatListModule,
    MatSnackBarModule
  ],
  templateUrl: './domain-details.component.html',
  styleUrl: './domain-details.component.scss'
})
export class DomainDetailsComponent {
  constructor(
    private dialogRef: MatDialogRef<DomainDetailsComponent>,
    private snackBar: MatSnackBar,
    @Inject(MAT_DIALOG_DATA) public domain: AcmeDomain
  ) {}

  copyToClipboard(text: string, label: string): void {
    navigator.clipboard.writeText(text).then(() => {
      this.snackBar.open(`${label} copied to clipboard`, 'Close', {
        duration: 2000
      });
    });
  }

  copyAllAsJson(): void {
    const jsonData = JSON.stringify({
      fulldomain: this.domain.fulldomain,
      subdomain: this.domain.subdomain,
      username: this.domain.username,
      password: this.domain.password,
      allowfrom: this.domain.allowfrom
    }, null, 2);
    
    navigator.clipboard.writeText(jsonData).then(() => {
      this.snackBar.open('All credentials copied as JSON', 'Close', {
        duration: 2000
      });
    });
  }

  copyAsCertbotCommand(): void {
    const command = `certbot certonly --manual \\
  --preferred-challenges dns \\
  --manual-auth-hook "acme-dns-auth.py" \\
  --manual-cleanup-hook "acme-dns-cleanup.py" \\
  -d yourdomain.com \\
  -d *.yourdomain.com

# ACME-DNS Credentials:
# Username: ${this.domain.username}
# Password: ${this.domain.password}
# Fulldomain: ${this.domain.fulldomain}`;
    
    navigator.clipboard.writeText(command).then(() => {
      this.snackBar.open('Certbot command copied', 'Close', {
        duration: 2000
      });
    });
  }

  close(): void {
    this.dialogRef.close();
  }
}