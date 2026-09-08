import { Injectable, inject } from '@angular/core';
import { MatSnackBar } from '@angular/material/snack-bar';

/** Copies text to the clipboard and confirms it, with a fallback for non-secure origins. */
@Injectable({ providedIn: 'root' })
export class CopyService {
  private snackBar = inject(MatSnackBar);

  async copy(text: string, label = 'Kopiert'): Promise<void> {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
      } else {
        this.fallbackCopy(text);
      }
      this.snackBar.open(label, undefined, { duration: 1800 });
    } catch {
      this.snackBar.open('Kopieren fehlgeschlagen — bitte manuell markieren', 'OK', {
        duration: 4000,
      });
    }
  }

  /** document.execCommand is deprecated but still the only option over plain HTTP. */
  private fallbackCopy(text: string): void {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    document.execCommand('copy');
    document.body.removeChild(area);
  }
}
