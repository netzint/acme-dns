import { Component, inject, input } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatTooltipModule } from '@angular/material/tooltip';
import { CopyService } from '../../core/copy.service';

/** A preformatted, copyable block of configuration text. */
@Component({
  selector: 'app-copy-block',
  imports: [MatButtonModule, MatIconModule, MatTooltipModule],
  template: `
    @if (label()) {
      <p class="block-label muted">{{ label() }}</p>
    }
    <div class="block-wrap">
      <pre class="code-block">{{ text() }}</pre>
      <button
        mat-icon-button
        class="copy-button"
        type="button"
        matTooltip="In die Zwischenablage kopieren"
        (click)="copy.copy(text(), copiedLabel())"
      >
        <mat-icon>content_copy</mat-icon>
      </button>
    </div>
  `,
  styles: [
    `
      :host {
        display: block;
      }

      .block-label {
        margin: 0 0 6px;
        font-size: 12px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }

      /* The button lives outside the scrolling <pre> so it stays put. */
      .block-wrap {
        position: relative;
      }

      .copy-button {
        position: absolute;
        top: 4px;
        right: 4px;
        background: var(--bg-inset);
      }
    `,
  ],
})
export class CopyBlockComponent {
  readonly text = input.required<string>();
  readonly label = input<string>('');
  readonly copiedLabel = input<string>('Kopiert');

  protected copy = inject(CopyService);
}
