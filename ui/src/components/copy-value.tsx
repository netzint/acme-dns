import { Copy } from 'lucide-react'
import { copyToClipboard } from '@/lib/clipboard'
import { cn } from '@/lib/utils'

interface CopyValueProps {
  value: string
  /** What to display, when it differs from what gets copied (masked passwords). */
  display?: string
  copiedLabel?: string
  className?: string
}

/** A single value that doubles as its own copy button. */
export function CopyValue({ value, display, copiedLabel = 'Kopiert', className }: CopyValueProps) {
  return (
    <button
      type="button"
      onClick={() => copyToClipboard(value, copiedLabel)}
      title="Kopieren"
      className={cn(
        'group bg-muted/40 hover:border-primary hover:bg-primary/5 inline-flex max-w-full items-center gap-2 rounded-md border px-2.5 py-1.5 text-left font-mono text-xs transition-colors [overflow-wrap:anywhere]',
        className,
      )}
    >
      <span className="min-w-0">{display ?? value}</span>
      <Copy className="text-muted-foreground group-hover:text-primary size-3.5 shrink-0" />
    </button>
  )
}
