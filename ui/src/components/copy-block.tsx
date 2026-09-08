import { Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { copyToClipboard } from '@/lib/clipboard'
import { cn } from '@/lib/utils'

interface CopyBlockProps {
  text: string
  label?: string
  copiedLabel?: string
  className?: string
}

/**
 * A preformatted, copyable block of configuration text. The button sits outside
 * the scrolling <pre> so it never covers the content.
 */
export function CopyBlock({ text, label, copiedLabel = 'Kopiert', className }: CopyBlockProps) {
  return (
    <div className={cn('space-y-1.5', className)}>
      {label && (
        <p className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
          {label}
        </p>
      )}
      <div className="relative">
        <pre className="bg-muted/40 text-foreground max-h-80 overflow-auto rounded-lg border p-3 pr-11 font-mono text-xs leading-relaxed whitespace-pre-wrap [overflow-wrap:anywhere]">
          {text}
        </pre>
        <Button
          size="icon"
          variant="ghost"
          className="absolute top-1.5 right-1.5 size-7"
          onClick={() => copyToClipboard(text, copiedLabel)}
          aria-label="In die Zwischenablage kopieren"
        >
          <Copy className="size-3.5" />
        </Button>
      </div>
    </div>
  )
}
